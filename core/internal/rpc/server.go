// Package rpc is the JSON-RPC 2.0 server the sidecar speaks over stdio.
// The extension is the only client. All methods are namespaced as
// "<service>/<verb>" (e.g. "chat/stream", "agent/run").
package rpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"

	"github.com/patmeh1/foundry-copilot/core/internal/agent"
	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/tools"
)

// Server bundles the dependencies the RPC handlers need.
type Server struct {
	Log     *slog.Logger
	Foundry *foundry.Client // nil-allowed; populated once endpoint is set
	Version string
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
	Paths []string `json:"paths"` // absolute paths to (re)index
}
type IndexRefreshReply struct {
	Indexed int `json:"indexed"`
}

// IndexRefresh re-indexes the given paths. Phase 7 will wire chromem-go.
func (s *Server) IndexRefresh(ctx context.Context, p IndexRefreshParams) (IndexRefreshReply, error) {
	return IndexRefreshReply{Indexed: 0}, nil
}
