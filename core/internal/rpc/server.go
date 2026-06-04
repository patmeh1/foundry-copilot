// Package rpc is the JSON-RPC 2.0 server the sidecar speaks over stdio.
// The extension is the only client. All methods are namespaced as
// "<service>/<verb>" (e.g. "chat/stream", "agent/run").
package rpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"

	"github.com/patmeh1/foundry-copilot/core/internal/agent"
	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/mcpx"
	"github.com/patmeh1/foundry-copilot/core/internal/rag"
	"github.com/patmeh1/foundry-copilot/core/internal/tools"
)

// Server bundles the dependencies the RPC handlers need.
type Server struct {
	Log     *slog.Logger
	Foundry *foundry.Client // nil-allowed; populated once endpoint is set
	Version string

	storeMu sync.Mutex
	store   *rag.Store

	mcpOnce sync.Once
	mcp     *mcpx.Manager
}

// mcpMgr returns the lazily-initialised MCP manager.
func (s *Server) mcpMgr() *mcpx.Manager {
	s.mcpOnce.Do(func() { s.mcp = mcpx.New() })
	return s.mcp
}

// ragStore returns the lazily-initialised vector store, opened from
// cfg.RAGIndexDir. Safe for concurrent use.
func (s *Server) ragStore() (*rag.Store, error) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	if s.store != nil {
		return s.store, nil
	}
	dir := config.Get().RAGIndexDir
	if dir == "" {
		return nil, fmt.Errorf("rag: index dir not configured")
	}
	store, err := rag.Open(filepath.Join(dir, "store.gob"))
	if err != nil {
		return nil, err
	}
	s.store = store
	return store, nil
}

// Run starts the JSON-RPC server on the provided in/out streams (typically
// os.Stdin / os.Stdout). It blocks until the connection is closed.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.WriteCloser) error {
	ch := channel.LSP(in, out)
	opts := &jrpc2.ServerOptions{
		Logger:      func(text string) { s.Log.Debug(text) },
		Concurrency: 8,
		AllowPush:   true, // Phase 3+ pushes chat/chunk notifications.
	}
	srv := jrpc2.NewServer(s.methods(), opts).Start(ch)
	done := make(chan error, 1)
	go func() { done <- srv.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		srv.Stop()
		<-done
		return ctx.Err()
	}
}

func (s *Server) methods() handler.Map {
	return handler.Map{
		"ping":            handler.New(s.Ping),
		"version":         handler.New(s.Version_),
		"config/set":      handler.New(s.ConfigSet),
		"config/get":      handler.New(s.ConfigGet),
		"chat/start":      handler.New(s.ChatStart),
		"chat/stream":     handler.New(s.ChatStream),
		"complete/inline": handler.New(s.CompleteInline),
		"agent/run":       handler.New(s.AgentRun),
		"index/refresh":   handler.New(s.IndexRefresh),
		"index/query":     handler.New(s.IndexQuery),
		"mcp/connect":     handler.New(s.MCPConnect),
		"mcp/disconnect":  handler.New(s.MCPDisconnect),
		"mcp/list":        handler.New(s.MCPList),
		"mcp/list_tools":  handler.New(s.MCPListTools),
		"mcp/call_tool":   handler.New(s.MCPCallTool),
	}
}

// ─── method types ───────────────────────────────────────────────────────────

type PingReply struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
}

func (s *Server) Ping(ctx context.Context) (PingReply, error) {
	return PingReply{OK: true, Version: s.Version}, nil
}

func (s *Server) Version_(ctx context.Context) (string, error) {
	return s.Version, nil
}

// ConfigSet receives a partial config from the extension and merges it in.
func (s *Server) ConfigSet(ctx context.Context, in config.Config) (config.Config, error) {
	// Endpoint changes must re-validate against the hard lock — even though
	// the package settings UI also validates, we trust nothing.
	if in.Endpoint != "" {
		if err := foundry.ValidateEndpoint(in.Endpoint); err != nil {
			return config.Config{}, err
		}
	}
	out := config.Update(in)
	s.Log.Info("config updated", "endpoint", out.Endpoint)
	return out, nil
}

func (s *Server) ConfigGet(ctx context.Context) (config.Config, error) {
	return config.Get(), nil
}

// ─── chat/stream ────────────────────────────────────────────────────────────

type ChatStreamParams struct {
	Deployment  string                `json:"deployment,omitempty"`
	Messages    []foundry.ChatMessage `json:"messages"`
	Temperature *float32              `json:"temperature,omitempty"`
	MaxTokens   *int32                `json:"max_tokens,omitempty"`
}

type ChatStreamReply struct {
	Chunks []foundry.ChatChunk `json:"chunks"`
}

// ChatStream is the simple (non-streamed) shape used by tests; the real
// streaming path is implemented via JSON-RPC notifications in Phase 3.
func (s *Server) ChatStream(ctx context.Context, p ChatStreamParams) (ChatStreamReply, error) {
	if s.Foundry == nil {
		return ChatStreamReply{}, fmt.Errorf("foundry client not configured (set endpoint via config/set)")
	}
	dep := p.Deployment
	if dep == "" {
		dep = config.Get().ChatDeployment
	}
	if dep == "" {
		return ChatStreamReply{}, fmt.Errorf("no chat deployment configured")
	}
	var chunks []foundry.ChatChunk
	err := s.Foundry.Chat(ctx, foundry.ChatRequest{
		Deployment:  dep,
		Messages:    p.Messages,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		Stream:      true,
	}, func(c foundry.ChatChunk) error {
		chunks = append(chunks, c)
		return nil
	})
	if err != nil {
		return ChatStreamReply{}, err
	}
	return ChatStreamReply{Chunks: chunks}, nil
}

// ─── chat/start (streaming via notifications) ──────────────────────────────

type ChatStartParams struct {
	StreamID    string                `json:"stream_id"`
	Deployment  string                `json:"deployment,omitempty"`
	Messages    []foundry.ChatMessage `json:"messages"`
	Temperature *float32              `json:"temperature,omitempty"`
	MaxTokens   *int32                `json:"max_tokens,omitempty"`
}

type ChatStartReply struct {
	StreamID     string `json:"stream_id"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type ChatChunkNotification struct {
	StreamID     string `json:"stream_id"`
	Delta        string `json:"delta,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	Err          string `json:"error,omitempty"`
}

// ChatStart kicks off a streaming chat call. Chunks are pushed as
// "chat/chunk" notifications tagged with the same StreamID. The reply
// returns once the stream terminates (finish_reason from the last chunk).
func (s *Server) ChatStart(ctx context.Context, p ChatStartParams) (ChatStartReply, error) {
	if s.Foundry == nil {
		return ChatStartReply{}, fmt.Errorf("foundry client not configured (set endpoint via config/set)")
	}
	if p.StreamID == "" {
		return ChatStartReply{}, fmt.Errorf("stream_id required")
	}
	dep := p.Deployment
	if dep == "" {
		dep = config.Get().ChatDeployment
	}
	if dep == "" {
		return ChatStartReply{}, fmt.Errorf("no chat deployment configured")
	}
	srv := jrpc2.ServerFromContext(ctx)
	var lastFinish string
	err := s.Foundry.Chat(ctx, foundry.ChatRequest{
		Deployment:  dep,
		Messages:    p.Messages,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		Stream:      true,
	}, func(c foundry.ChatChunk) error {
		if c.FinishReason != "" {
			lastFinish = c.FinishReason
		}
		return srv.Notify(ctx, "chat/chunk", ChatChunkNotification{
			StreamID:     p.StreamID,
			Delta:        c.Delta,
			FinishReason: c.FinishReason,
			Err:          c.Err,
		})
	})
	if err != nil {
		_ = srv.Notify(ctx, "chat/chunk", ChatChunkNotification{
			StreamID: p.StreamID, Err: err.Error(), FinishReason: "error",
		})
		return ChatStartReply{}, err
	}
	return ChatStartReply{StreamID: p.StreamID, FinishReason: lastFinish}, nil
}

// ───
// ─── stubs for future phases ───────────────────────────────────────────────

type CompleteInlineParams struct {
	Prefix   string `json:"prefix"`
	Suffix   string `json:"suffix"`
	Language string `json:"language"`
}
type CompleteInlineReply struct {
	Text string `json:"text"`
}

// CompleteInline returns a single FIM-style completion. Uses chat with a
// tightly-scoped system prompt so it works with any modern chat model
// (no FIM-specific deployment required).
func (s *Server) CompleteInline(ctx context.Context, p CompleteInlineParams) (CompleteInlineReply, error) {
	if s.Foundry == nil {
		return CompleteInlineReply{}, fmt.Errorf("foundry client not configured")
	}
	cfg := config.Get()
	dep := cfg.CompletionDeployment
	if dep == "" {
		dep = cfg.ChatDeployment
	}
	if dep == "" {
		return CompleteInlineReply{}, fmt.Errorf("no completion or chat deployment configured")
	}
	lang := p.Language
	if lang == "" {
		lang = "plaintext"
	}
	sys := "You are a code completion engine. Given a code prefix and suffix, " +
		"output ONLY the code that goes BETWEEN them. Output the completion text only, " +
		"no markdown fences, no commentary, no quotes. Match the language and indentation."
	user := fmt.Sprintf("LANGUAGE: %s\n\n<PREFIX>\n%s\n</PREFIX>\n\n<SUFFIX>\n%s\n</SUFFIX>\n\nCompletion:",
		lang, p.Prefix, p.Suffix)
	out, err := s.Foundry.Complete(ctx, dep, []foundry.ChatMessage{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}, 256)
	if err != nil {
		return CompleteInlineReply{}, err
	}
	return CompleteInlineReply{Text: sanitizeCompletion(out)}, nil
}

// sanitizeCompletion strips common artifacts (markdown fences, surrounding
// whitespace) the model sometimes emits even when asked not to.
func sanitizeCompletion(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl > 0 {
			s = s[nl+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

type AgentRunParams struct {
	StreamID string `json:"stream_id"`
	Task     string `json:"task"`
	Workdir  string `json:"workdir,omitempty"`
	System   string `json:"system,omitempty"`
	MaxSteps int    `json:"max_steps,omitempty"`
}
type AgentRunReply struct {
	StreamID     string `json:"stream_id"`
	FinalMessage string `json:"final_message"`
	StepsTaken   int    `json:"steps_taken"`
}

type AgentEventNotification struct {
	StreamID   string `json:"stream_id"`
	Kind       string `json:"kind"`
	Step       int    `json:"step"`
	Text       string `json:"text,omitempty"`
	ToolName   string `json:"tool,omitempty"`
	ToolArgs   string `json:"args,omitempty"`
	ToolResult string `json:"result,omitempty"`
	ToolError  string `json:"tool_error,omitempty"`
}

// AgentRun runs the agent loop. Events are pushed to the client as
// "agent/event" notifications tagged with the StreamID; the reply is
// returned once the loop terminates.
func (s *Server) AgentRun(ctx context.Context, p AgentRunParams) (AgentRunReply, error) {
	if s.Foundry == nil {
		return AgentRunReply{}, fmt.Errorf("foundry client not configured")
	}
	if p.StreamID == "" {
		return AgentRunReply{}, fmt.Errorf("stream_id required")
	}
	if strings.TrimSpace(p.Task) == "" {
		return AgentRunReply{}, fmt.Errorf("task required")
	}
	cfg := config.Get()
	root := p.Workdir
	if root == "" {
		root = cfg.WorkspaceRoot
	}
	if root == "" {
		return AgentRunReply{}, fmt.Errorf("workdir required (or set workspace_root via config/set)")
	}
	dep := cfg.ChatDeployment
	if dep == "" {
		return AgentRunReply{}, fmt.Errorf("no chat deployment configured")
	}
	maxSteps := p.MaxSteps
	if maxSteps <= 0 {
		maxSteps = cfg.AgentMaxSteps
	}
	if maxSteps <= 0 {
		maxSteps = 12
	}
	reg := tools.NewRegistry()
	reg.Register(tools.FSRead{Root: root})
	reg.Register(tools.CodeSearch{Root: root})
	reg.Register(tools.FSWrite{Root: root, Allow: cfg.AgentAllowWrite})
	reg.Register(tools.Shell{Root: root, Allow: cfg.AgentAllowShell})
	// Only expose rag_search if the index has been built and an embedding
	// deployment is configured. The store opens lazily and reports Len()==0
	// when empty, in which case we omit the tool entirely.
	if cfg.EmbeddingDeployment != "" {
		if store, err := s.ragStore(); err == nil && store.Len() > 0 {
			reg.Register(tools.RAGSearch{
				Foundry: s.Foundry, Store: store, Deployment: cfg.EmbeddingDeployment,
			})
		}
	}
	// Surface every tool from each connected MCP server. Adapter handles
	// name namespacing (mcp__{server}__{tool}).
	if mgr := s.mcpMgr(); mgr != nil {
		for _, info := range mgr.ListAllTools(ctx) {
			reg.Register(tools.MCPTool{
				Manager:  mgr,
				ServerID: info.ServerID,
				ToolName: info.Name,
				Desc:     info.Description,
				Schema:   info.InputSchema,
			})
		}
	}

	loop := &agent.Loop{
		Foundry:    s.Foundry,
		Tools:      reg,
		Deployment: dep,
		System:     p.System,
		MaxSteps:   maxSteps,
	}
	srv := jrpc2.ServerFromContext(ctx)
	final, steps, err := loop.Run(ctx, p.Task, func(e agent.Event) {
		_ = srv.Notify(ctx, "agent/event", AgentEventNotification{
			StreamID:   p.StreamID,
			Kind:       e.Kind,
			Step:       e.Step,
			Text:       e.Text,
			ToolName:   e.ToolName,
			ToolArgs:   e.ToolArgs,
			ToolResult: e.ToolResult,
			ToolError:  e.ToolError,
		})
	})
	if err != nil {
		return AgentRunReply{StreamID: p.StreamID, FinalMessage: final, StepsTaken: steps}, err
	}
	return AgentRunReply{StreamID: p.StreamID, FinalMessage: final, StepsTaken: steps}, nil
}

type IndexRefreshParams struct {
	Paths    []string `json:"paths,omitempty"`     // optional explicit list (workspace-relative or absolute)
	MaxFiles int      `json:"max_files,omitempty"` // cap; default 4000
}
type IndexRefreshReply struct {
	Files   int `json:"files"`
	Chunks  int `json:"chunks"`
	Skipped int `json:"skipped"`
}

// IndexRefresh rebuilds the workspace index. If Paths is empty the entire
// workspace_root is walked (with the standard noise-dir denylist).
func (s *Server) IndexRefresh(ctx context.Context, p IndexRefreshParams) (IndexRefreshReply, error) {
	if s.Foundry == nil {
		return IndexRefreshReply{}, fmt.Errorf("foundry client not configured")
	}
	cfg := config.Get()
	if cfg.EmbeddingDeployment == "" {
		return IndexRefreshReply{}, fmt.Errorf("embedding_deployment required")
	}
	if cfg.WorkspaceRoot == "" {
		return IndexRefreshReply{}, fmt.Errorf("workspace_root required")
	}
	store, err := s.ragStore()
	if err != nil {
		return IndexRefreshReply{}, err
	}
	files := p.Paths
	if len(files) == 0 {
		max := p.MaxFiles
		if max <= 0 {
			max = 4000
		}
		files, err = rag.WalkWorkspace(ctx, cfg.WorkspaceRoot, max)
		if err != nil {
			return IndexRefreshReply{}, err
		}
	}
	// Chunk first.
	var allChunks []rag.Chunk
	skipped := 0
	for _, rel := range files {
		full := rel
		if !filepath.IsAbs(full) {
			full = filepath.Join(cfg.WorkspaceRoot, rel)
		} else {
			r, err := filepath.Rel(cfg.WorkspaceRoot, full)
			if err == nil {
				rel = r
			}
		}
		cs, err := rag.ChunkFile(full, rel, rag.ChunkOptions{})
		if err != nil {
			skipped++
			continue
		}
		allChunks = append(allChunks, cs...)
	}
	// Embed in batches of 64.
	const batch = 64
	for start := 0; start < len(allChunks); start += batch {
		if err := ctx.Err(); err != nil {
			return IndexRefreshReply{}, err
		}
		end := start + batch
		if end > len(allChunks) {
			end = len(allChunks)
		}
		inputs := make([]string, end-start)
		for i := start; i < end; i++ {
			inputs[i-start] = allChunks[i].Text
		}
		vecs, err := s.Foundry.Embed(ctx, cfg.EmbeddingDeployment, inputs)
		if err != nil {
			return IndexRefreshReply{}, fmt.Errorf("embed batch %d: %w", start/batch, err)
		}
		for i := range vecs {
			allChunks[start+i].Vec = vecs[i]
		}
	}
	if err := store.Replace(allChunks); err != nil {
		return IndexRefreshReply{}, err
	}
	s.Log.Info("index refresh complete",
		"files", len(files), "chunks", len(allChunks), "skipped", skipped)
	return IndexRefreshReply{Files: len(files), Chunks: len(allChunks), Skipped: skipped}, nil
}

type IndexQueryParams struct {
	Query string `json:"query"`
	K     int    `json:"k,omitempty"` // default 8
}
type IndexHit struct {
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Text      string  `json:"text"`
	Score     float32 `json:"score"`
}
type IndexQueryReply struct {
	Hits []IndexHit `json:"hits"`
}

// IndexQuery embeds the query and returns the top-K most similar chunks.
func (s *Server) IndexQuery(ctx context.Context, p IndexQueryParams) (IndexQueryReply, error) {
	if s.Foundry == nil {
		return IndexQueryReply{}, fmt.Errorf("foundry client not configured")
	}
	if strings.TrimSpace(p.Query) == "" {
		return IndexQueryReply{}, fmt.Errorf("query required")
	}
	cfg := config.Get()
	if cfg.EmbeddingDeployment == "" {
		return IndexQueryReply{}, fmt.Errorf("embedding_deployment required")
	}
	k := p.K
	if k <= 0 {
		k = 8
	}
	store, err := s.ragStore()
	if err != nil {
		return IndexQueryReply{}, err
	}
	vecs, err := s.Foundry.Embed(ctx, cfg.EmbeddingDeployment, []string{p.Query})
	if err != nil || len(vecs) == 0 {
		return IndexQueryReply{}, fmt.Errorf("embed query: %v", err)
	}
	hits := store.Search(vecs[0], k)
	out := IndexQueryReply{Hits: make([]IndexHit, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, IndexHit{
			Path: h.Chunk.Path, StartLine: h.Chunk.StartLine, EndLine: h.Chunk.EndLine,
			Text: h.Chunk.Text, Score: h.Score,
		})
	}
	return out, nil
}

// ─── MCP ──────────────────────────────────────────────────────────────────

type MCPConnectParams struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
type MCPConnectReply struct {
	ID    string `json:"id"`
	Tools int    `json:"tools"`
}

func (s *Server) MCPConnect(ctx context.Context, p MCPConnectParams) (MCPConnectReply, error) {
	if err := s.mcpMgr().Connect(ctx, mcpx.ServerSpec{
		ID: p.ID, Command: p.Command, Args: p.Args, Env: p.Env,
	}); err != nil {
		return MCPConnectReply{}, err
	}
	tools, _ := s.mcpMgr().ListTools(ctx, p.ID)
	s.Log.Info("mcp connect", "id", p.ID, "tools", len(tools))
	return MCPConnectReply{ID: p.ID, Tools: len(tools)}, nil
}

type MCPDisconnectParams struct {
	ID string `json:"id"`
}
type MCPDisconnectReply struct {
	OK bool `json:"ok"`
}

func (s *Server) MCPDisconnect(ctx context.Context, p MCPDisconnectParams) (MCPDisconnectReply, error) {
	if err := s.mcpMgr().Disconnect(p.ID); err != nil {
		return MCPDisconnectReply{}, err
	}
	return MCPDisconnectReply{OK: true}, nil
}

type MCPListReply struct {
	Servers []string `json:"servers"`
}

func (s *Server) MCPList(ctx context.Context) (MCPListReply, error) {
	return MCPListReply{Servers: s.mcpMgr().List()}, nil
}

type MCPListToolsParams struct {
	ID string `json:"id"`
}
type MCPListToolsTool struct {
	ServerID    string         `json:"server_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}
type MCPListToolsReply struct {
	Tools []MCPListToolsTool `json:"tools"`
}

func (s *Server) MCPListTools(ctx context.Context, p MCPListToolsParams) (MCPListToolsReply, error) {
	var infos []mcpx.ToolInfo
	if p.ID != "" {
		var err error
		infos, err = s.mcpMgr().ListTools(ctx, p.ID)
		if err != nil {
			return MCPListToolsReply{}, err
		}
	} else {
		infos = s.mcpMgr().ListAllTools(ctx)
	}
	out := MCPListToolsReply{Tools: make([]MCPListToolsTool, 0, len(infos))}
	for _, t := range infos {
		out.Tools = append(out.Tools, MCPListToolsTool{
			ServerID: t.ServerID, Name: t.Name,
			Description: t.Description, InputSchema: t.InputSchema,
		})
	}
	return out, nil
}

type MCPCallToolParams struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}
type MCPCallToolReply struct {
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

func (s *Server) MCPCallTool(ctx context.Context, p MCPCallToolParams) (MCPCallToolReply, error) {
	text, err := s.mcpMgr().CallTool(ctx, p.ID, p.Name, p.Args)
	r := MCPCallToolReply{Text: text}
	if err != nil {
		r.Error = err.Error()
	}
	return r, nil
}
