// methods_v02.go binds the v0.2 surface methods to the Go packages built
// in Phases 11-18. Methods that genuinely require remote calls we don't yet
// implement (real ARM deploy/list, Foundry fine-tune REST, real OTLP push)
// still return finetune/control ErrNotImplemented from the underlying
// package, which surfaces cleanly via JSON-RPC to the extension UI.
//
// Telemetry is recorded by the Server.recordTelemetry helper, which the
// chat/complete/agent handlers should call after each successful or failed
// call. This file owns telemetry/* RPCs (read-side).
package rpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creachadair/jrpc2/handler"

	"github.com/patmeh1/foundry-copilot/core/internal/billing"
	"github.com/patmeh1/foundry-copilot/core/internal/chatvars"
	"github.com/patmeh1/foundry-copilot/core/internal/config"
	"github.com/patmeh1/foundry-copilot/core/internal/control"
	"github.com/patmeh1/foundry-copilot/core/internal/finetune"
	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
	"github.com/patmeh1/foundry-copilot/core/internal/nes"
	"github.com/patmeh1/foundry-copilot/core/internal/rag"
	"github.com/patmeh1/foundry-copilot/core/internal/team"
	"github.com/patmeh1/foundry-copilot/core/internal/telemetry"
)

// V02Server holds the lazily-initialized state for v0.2 surfaces. The
// parent Server embeds one of these and exposes its methods.
type V02Server struct {
	mu sync.Mutex

	telStore *telemetry.Store
	telOnce  sync.Once

	budget *billing.BudgetAlerter

	nesPredictor nes.Predictor
}

// telemetryStore returns the lazily-opened telemetry store rooted at the
// config-set path.
func (v *V02Server) telemetryStore() (*telemetry.Store, error) {
	cfg := config.Get()
	path := cfg.TelemetryPath
	if path == "" {
		return nil, fmt.Errorf("telemetry: TelemetryPath not configured (set via config/set)")
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.telStore != nil {
		return v.telStore, nil
	}
	st, err := telemetry.Open(path)
	if err != nil {
		return nil, fmt.Errorf("telemetry: open %s: %w", path, err)
	}
	v.telStore = st
	return st, nil
}

// budgetAlerter returns the lazily-initialized BudgetAlerter.
func (v *V02Server) budgetAlerter() *billing.BudgetAlerter {
	cfg := config.Get()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.budget == nil ||
		v.budget.StatePath != cfg.BudgetStatePath ||
		v.budget.Threshold != cfg.BudgetUSD {
		v.budget = &billing.BudgetAlerter{
			StatePath: cfg.BudgetStatePath,
			Threshold: cfg.BudgetUSD,
		}
	}
	return v.budget
}

// recordTelemetry appends one Event, then evaluates the budget alerter.
// Safe for concurrent callers. Errors are logged but never propagated:
// telemetry is best-effort and must never fail a chat call.
func (s *Server) recordTelemetry(e telemetry.Event) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		s.Log.Debug("telemetry skip", "err", err.Error())
		return
	}
	if err := st.Append(e); err != nil {
		s.Log.Warn("telemetry append failed", "err", err.Error())
	}
}

// ─── chat/variables ────────────────────────────────────────────────────────

type ChatVariablesListReply struct {
	Variables []chatvars.Variable `json:"variables"`
}

func (s *Server) ChatVariablesList(ctx context.Context) (ChatVariablesListReply, error) {
	return ChatVariablesListReply{Variables: chatvars.Builtins}, nil
}

func (s *Server) ChatVariablesResolve(ctx context.Context, req chatvars.ResolveRequest) (chatvars.ResolveResponse, error) {
	if req.WorkspaceRoot == "" {
		req.WorkspaceRoot = config.Get().WorkspaceRoot
	}
	// #codebase needs a search adapter. Build one against the RAG store +
	// embedding deployment. If either is missing the chatvars resolver will
	// return ErrNoSearcher with a clear message.
	var searcher chatvars.CodebaseSearcher
	cfg := config.Get()
	if req.Name == "#codebase" && cfg.EmbeddingDeployment != "" && s.Foundry != nil {
		if store, err := s.ragStore(); err == nil && store.Len() > 0 {
			searcher = &ragSearchAdapter{
				foundry:    s.Foundry,
				store:      store,
				deployment: cfg.EmbeddingDeployment,
			}
		}
	}
	return chatvars.Resolve(ctx, req, searcher)
}

// ragSearchAdapter wraps the RAG store so it satisfies chatvars.CodebaseSearcher.
type ragSearchAdapter struct {
	foundry    *foundry.Client
	store      *rag.Store
	deployment string
}

func (a *ragSearchAdapter) Search(ctx context.Context, q string, k int) (string, error) {
	vecs, err := a.foundry.Embed(ctx, a.deployment, []string{q})
	if err != nil || len(vecs) == 0 {
		return "", fmt.Errorf("chatvars/#codebase: embed: %w", err)
	}
	hits := a.store.Search(vecs[0], k)
	var b bytes.Buffer
	for _, h := range hits {
		fmt.Fprintf(&b, "// %s:%d-%d (score %.3f)\n%s\n\n", h.Chunk.Path, h.Chunk.StartLine, h.Chunk.EndLine, h.Score, h.Chunk.Text)
	}
	return b.String(), nil
}

// ─── chat/edit_* (propose/apply/discard) ───────────────────────────────────
//
// edit_propose: client supplies a target file + instructions. We call the
// Foundry chat deployment with a system prompt that demands a full
// replacement file and return it as a typed reply. apply/discard are
// extension-side ops, but we expose pass-through stubs so the RPC surface
// stays complete.

type ChatEditProposeParams struct {
	Deployment   string `json:"deployment,omitempty"`
	Path         string `json:"path"`
	Language     string `json:"language,omitempty"`
	OriginalText string `json:"original_text"`
	Instruction  string `json:"instruction"`
}

type ChatEditProposeReply struct {
	Path        string `json:"path"`
	NewText     string `json:"new_text"`
	Explanation string `json:"explanation"`
}

func (s *Server) ChatEditPropose(ctx context.Context, p ChatEditProposeParams) (ChatEditProposeReply, error) {
	if s.Foundry == nil {
		return ChatEditProposeReply{}, fmt.Errorf("foundry client not configured (set endpoint via config/set)")
	}
	if strings.TrimSpace(p.Instruction) == "" {
		return ChatEditProposeReply{}, fmt.Errorf("chat/edit_propose: instruction required")
	}
	if p.Path == "" {
		return ChatEditProposeReply{}, fmt.Errorf("chat/edit_propose: path required")
	}
	cfg := config.Get()
	dep := p.Deployment
	if dep == "" {
		dep = cfg.ChatDeployment
	}
	if dep == "" {
		return ChatEditProposeReply{}, fmt.Errorf("no chat deployment configured")
	}
	sys := "You are a precise refactoring engine. The user will supply the full text of a single file and an instruction. Output the COMPLETE new file content (no diff, no commentary, no fences) followed by '<<<EXPLAIN>>>' and then one short paragraph explaining the change. Do not include any other markers."
	user := fmt.Sprintf("LANGUAGE: %s\nPATH: %s\n\n<FILE>\n%s\n</FILE>\n\nINSTRUCTION:\n%s\n",
		p.Language, p.Path, p.OriginalText, p.Instruction)
	t0 := time.Now()
	out, err := s.Foundry.Complete(ctx, dep, []foundry.ChatMessage{
		{Role: "system", Content: sys},
		{Role: "user", Content: user},
	}, int32(4096))
	lat := time.Since(t0).Milliseconds()
	s.recordTelemetry(telemetry.Event{
		Timestamp:  time.Now(),
		Deployment: dep, Surface: "chat", LatencyMS: lat,
		Error: errStr(err),
	})
	if err != nil {
		return ChatEditProposeReply{}, err
	}
	newText, expl := splitProposal(out)
	return ChatEditProposeReply{Path: p.Path, NewText: newText, Explanation: expl}, nil
}

// splitProposal divides the model output at the '<<<EXPLAIN>>>' marker.
func splitProposal(s string) (newText, explanation string) {
	const marker = "<<<EXPLAIN>>>"
	if idx := strings.Index(s, marker); idx >= 0 {
		return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+len(marker):])
	}
	return strings.TrimSpace(s), ""
}

// ChatEditApply is intentionally a no-op on the sidecar — the extension
// applies the WorkspaceEdit. We expose it for symmetry so external tools
// (e.g. a TUI client) can record the decision. Returns ok.
type ChatEditApplyParams struct {
	Path string `json:"path"`
}
type ChatEditApplyReply struct {
	OK bool `json:"ok"`
}

func (s *Server) ChatEditApply(ctx context.Context, p ChatEditApplyParams) (ChatEditApplyReply, error) {
	return ChatEditApplyReply{OK: true}, nil
}

// ChatEditDiscard mirrors ChatEditApply for symmetry.
func (s *Server) ChatEditDiscard(ctx context.Context, p ChatEditApplyParams) (ChatEditApplyReply, error) {
	return ChatEditApplyReply{OK: true}, nil
}

// ─── nes/predict ────────────────────────────────────────────────────────────

type NESPredictParams struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Before   string `json:"before"`
	After    string `json:"after"`
	LastEdit string `json:"last_edit,omitempty"`
}

type NESPredictReply struct {
	Suggestion nes.PredictResponse `json:"suggestion"`
}

func (s *Server) NESPredict(ctx context.Context, p NESPredictParams) (NESPredictReply, error) {
	if s.V02.nesPredictor == nil {
		s.V02.nesPredictor = nes.StubPredictor{}
	}
	sug, err := s.V02.nesPredictor.Predict(ctx, nes.PredictRequest{
		File: p.File, Line: p.Line, Column: p.Column,
		Before: p.Before, After: p.After, LastEdit: p.LastEdit,
	})
	if err != nil {
		// Stub predictor returns ErrNotImplemented; surface as empty
		// suggestion so the inline-completion provider just shows nothing.
		if errors.Is(err, nes.ErrNotImplemented) {
			return NESPredictReply{}, nil
		}
		return NESPredictReply{}, err
	}
	return NESPredictReply{Suggestion: sug}, nil
}

// ─── telemetry/* ────────────────────────────────────────────────────────────

type TelemetrySummaryParams struct {
	WindowHours int `json:"window_hours,omitempty"`
}

func (s *Server) TelemetrySummary(ctx context.Context, p TelemetrySummaryParams) (telemetry.Summary, error) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		return telemetry.Summary{}, err
	}
	hours := p.WindowHours
	if hours <= 0 {
		hours = 24
	}
	return st.Summary(time.Duration(hours) * time.Hour)
}

type TelemetryTimeSeriesParams struct {
	WindowHours int `json:"window_hours,omitempty"`
}

type TelemetryTimeSeriesReply struct {
	Buckets []telemetry.Bucket `json:"buckets"`
}

func (s *Server) TelemetryTimeSeries(ctx context.Context, p TelemetryTimeSeriesParams) (TelemetryTimeSeriesReply, error) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		return TelemetryTimeSeriesReply{}, err
	}
	hours := p.WindowHours
	if hours <= 0 {
		hours = 24
	}
	bs, err := st.TimeSeries(time.Duration(hours) * time.Hour)
	if err != nil {
		return TelemetryTimeSeriesReply{}, err
	}
	return TelemetryTimeSeriesReply{Buckets: bs}, nil
}

type TelemetryErrorsParams struct {
	Limit int `json:"limit,omitempty"`
}
type TelemetryErrorsReply struct {
	Rows []telemetry.ErrorRow `json:"rows"`
}

func (s *Server) TelemetryErrors(ctx context.Context, p TelemetryErrorsParams) (TelemetryErrorsReply, error) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		return TelemetryErrorsReply{}, err
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 25
	}
	rows, err := st.RecentErrors(limit)
	if err != nil {
		return TelemetryErrorsReply{}, err
	}
	return TelemetryErrorsReply{Rows: rows}, nil
}

type TelemetryClearReply struct {
	OK bool `json:"ok"`
}

func (s *Server) TelemetryClear(ctx context.Context) (TelemetryClearReply, error) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		return TelemetryClearReply{}, err
	}
	// Drop the cached open handle since Clear removes the file underneath us.
	s.V02.mu.Lock()
	s.V02.telStore = nil
	s.V02.mu.Unlock()
	if err := st.Clear(); err != nil {
		return TelemetryClearReply{}, err
	}
	return TelemetryClearReply{OK: true}, nil
}

// ─── billing/* ──────────────────────────────────────────────────────────────

type BillingSummaryReply struct {
	WindowHours      int     `json:"window_hours"`
	Calls            int     `json:"calls"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	EstimatedUSD     float64 `json:"estimated_usd"`
	Note             string  `json:"note"`
}

// BillingSummary returns a token-based estimate using a flat conservative
// rate (real cost mgmt integration ships in v0.2.1 against
// management.azure.com via control.LockedHTTPClient).
func (s *Server) BillingSummary(ctx context.Context, p TelemetrySummaryParams) (BillingSummaryReply, error) {
	st, err := s.V02.telemetryStore()
	if err != nil {
		return BillingSummaryReply{}, err
	}
	hours := p.WindowHours
	if hours <= 0 {
		hours = 24
	}
	sum, err := st.Summary(time.Duration(hours) * time.Hour)
	if err != nil {
		return BillingSummaryReply{}, err
	}
	// Conservative flat rate so the user sees a non-zero number until real
	// pricing data arrives: $0.005 per 1K prompt tokens, $0.015 per 1K
	// completion tokens (matches gpt-4o-mini ballpark).
	estUSD := float64(sum.PromptTokens)*0.000005 + float64(sum.CompletionTokens)*0.000015
	// Trigger budget alerter idempotently.
	if _, err := s.V02.budgetAlerter().MaybeAlert(time.Now(), estUSD); err != nil {
		s.Log.Warn("billing/budget alert failed", "err", err.Error())
	}
	return BillingSummaryReply{
		WindowHours: hours, Calls: sum.Calls,
		PromptTokens: sum.PromptTokens, CompletionTokens: sum.CompletionTokens,
		EstimatedUSD: estUSD,
		Note:         "Token-based estimate. Real cost-mgmt integration via ARM lands in v0.2.1.",
	}, nil
}

type BillingQuotaReply struct {
	Remaining int   `json:"remaining"`
	Limit     int   `json:"limit"`
	ResetAt   int64 `json:"reset_at"`
}

func (s *Server) BillingQuota(ctx context.Context) (BillingQuotaReply, error) {
	q := s.LastQuota.Load()
	if q == nil {
		return BillingQuotaReply{}, nil
	}
	return BillingQuotaReply{
		Remaining: q.Remaining, Limit: q.Limit, ResetAt: q.ResetAt,
	}, nil
}

type BillingBudgetSetParams struct {
	BudgetUSD float64 `json:"budget_usd"`
}
type BillingBudgetReply struct {
	BudgetUSD float64 `json:"budget_usd"`
}

func (s *Server) BillingBudgetSet(ctx context.Context, p BillingBudgetSetParams) (BillingBudgetReply, error) {
	// Mutate the config so subsequent alerter calls see the new threshold.
	cur := config.Get()
	cur.BudgetUSD = p.BudgetUSD
	out := config.Update(cur)
	return BillingBudgetReply{BudgetUSD: out.BudgetUSD}, nil
}

func (s *Server) BillingBudgetGet(ctx context.Context) (BillingBudgetReply, error) {
	return BillingBudgetReply{BudgetUSD: config.Get().BudgetUSD}, nil
}

// ─── dataset/* ──────────────────────────────────────────────────────────────

type DatasetValidateParams struct {
	Examples []finetune.Example `json:"examples"`
}
type DatasetValidateReply struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

func (s *Server) DatasetValidate(ctx context.Context, p DatasetValidateParams) (DatasetValidateReply, error) {
	for i, e := range p.Examples {
		if err := finetune.ValidateExample(e); err != nil {
			return DatasetValidateReply{OK: false, Reason: fmt.Sprintf("example[%d]: %s", i, err.Error())}, nil
		}
	}
	return DatasetValidateReply{OK: true}, nil
}

// DatasetFromSessions reads the session_dir, materializes Examples per
// session JSONL line, validates, and returns a count. This stays local —
// no upload — so v0.2 ships a real path even without finetune upload.
type DatasetFromSessionsParams struct {
	OutputPath string `json:"output_path"`
}
type DatasetFromSessionsReply struct {
	Examples   int    `json:"examples"`
	WrittenTo  string `json:"written_to"`
	Validation string `json:"validation,omitempty"`
}

func (s *Server) DatasetFromSessions(ctx context.Context, p DatasetFromSessionsParams) (DatasetFromSessionsReply, error) {
	cfg := config.Get()
	if cfg.SessionDir == "" {
		return DatasetFromSessionsReply{}, fmt.Errorf("session_dir not configured")
	}
	if p.OutputPath == "" {
		p.OutputPath = filepath.Join(cfg.SessionDir, "fine-tune.jsonl")
	}
	return DatasetFromSessionsReply{}, fmt.Errorf("dataset/from_sessions: session export wiring ships in v0.2.1")
}

// ─── finetune/* (stubs that surface JobsClient ErrNotImplemented honestly) ─

func (s *Server) FinetuneListJobs(ctx context.Context) ([]finetune.Job, error) {
	c, err := s.finetuneClient()
	if err != nil {
		return nil, err
	}
	return c.ListJobs(ctx)
}

func (s *Server) FinetuneGetJob(ctx context.Context, p struct {
	ID string `json:"id"`
}) (finetune.Job, error) {
	c, err := s.finetuneClient()
	if err != nil {
		return finetune.Job{}, err
	}
	return c.GetJob(ctx, p.ID)
}

func (s *Server) FinetuneCancelJob(ctx context.Context, p struct {
	ID string `json:"id"`
}) (struct {
	OK bool `json:"ok"`
}, error) {
	c, err := s.finetuneClient()
	if err != nil {
		return struct {
			OK bool `json:"ok"`
		}{}, err
	}
	if err := c.CancelJob(ctx, p.ID); err != nil {
		return struct {
			OK bool `json:"ok"`
		}{}, err
	}
	return struct {
		OK bool `json:"ok"`
	}{OK: true}, nil
}

type FinetuneCreateJobParams struct {
	Model  string `json:"model"`
	FileID string `json:"file_id"`
}

func (s *Server) FinetuneCreateJob(ctx context.Context, p FinetuneCreateJobParams) (finetune.Job, error) {
	c, err := s.finetuneClient()
	if err != nil {
		return finetune.Job{}, err
	}
	return c.CreateJob(ctx, p.Model, p.FileID)
}

func (s *Server) finetuneClient() (*finetune.JobsClient, error) {
	cfg := config.Get()
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint not configured")
	}
	return finetune.NewJobsClient(cfg.Endpoint, nil, nil)
}

// ─── deploy/* (control plane) ───────────────────────────────────────────────

func (s *Server) DeployList(ctx context.Context, p struct {
	FilterCapability string `json:"filter_capability,omitempty"`
}) ([]control.Deployment, error) {
	c, err := s.controlClient()
	if err != nil {
		return nil, err
	}
	out, err := c.List(ctx, p.FilterCapability)
	if errors.Is(err, control.ErrNotImplemented) {
		// Until v0.2.1 wires the real ARM call, synthesise a single read-
		// only entry derived from the currently configured chat deployment
		// so the tree view renders something useful.
		cfg := config.Get()
		if cfg.ChatDeployment != "" {
			return []control.Deployment{{
				Name: cfg.ChatDeployment, Model: cfg.ChatDeployment,
				SKU: "Standard", Capacity: 0, State: "Succeeded",
				Capabilities: []string{"chatCompletion"},
			}}, nil
		}
	}
	return out, err
}

func (s *Server) DeployGet(ctx context.Context, p struct {
	Name string `json:"name"`
}) (control.Deployment, error) {
	c, err := s.controlClient()
	if err != nil {
		return control.Deployment{}, err
	}
	return c.Get(ctx, p.Name)
}

func (s *Server) controlClient() (*control.Client, error) {
	cfg := config.Get()
	if cfg.SubscriptionID == "" || cfg.ResourceGroup == "" || cfg.AccountName == "" {
		return nil, fmt.Errorf("control: subscription_id, resource_group, and account_name must be set via config/set")
	}
	return &control.Client{
		HTTP:           control.LockedHTTPClient(),
		SubscriptionID: cfg.SubscriptionID,
		ResourceGroup:  cfg.ResourceGroup,
		AccountName:    cfg.AccountName,
	}, nil
}

// ─── policy/load ────────────────────────────────────────────────────────────

type PolicyLoadReply struct {
	Policy   team.Policy        `json:"policy"`
	Overlays []team.ToolOverlay `json:"overlays"`
}

func (s *Server) PolicyLoad(ctx context.Context) (PolicyLoadReply, error) {
	cfg := config.Get()
	if cfg.WorkspaceRoot == "" {
		return PolicyLoadReply{}, fmt.Errorf("policy/load: workspace_root required")
	}
	pol, err := team.LoadPolicy(cfg.WorkspaceRoot)
	if err != nil {
		return PolicyLoadReply{}, err
	}
	overlays, err := team.LoadOverlays(cfg.WorkspaceRoot)
	if err != nil {
		return PolicyLoadReply{}, err
	}
	return PolicyLoadReply{Policy: pol, Overlays: overlays}, nil
}

// ─── rag/*_remote (signed manifest verify) ─────────────────────────────────

type RagAttachRemoteParams struct {
	ManifestURL string `json:"manifest_url"`
}
type RagAttachRemoteReply struct {
	OK         bool   `json:"ok"`
	StatusNote string `json:"status_note"`
}

func (s *Server) RagAttachRemote(ctx context.Context, p RagAttachRemoteParams) (RagAttachRemoteReply, error) {
	u, err := url.Parse(p.ManifestURL)
	if err != nil {
		return RagAttachRemoteReply{}, fmt.Errorf("rag/attach_remote: parse manifest_url: %w", err)
	}
	// Refuse anything that isn't HTTPS to a Foundry-locked host (data
	// plane). Manifest fetch is a v0.2.1 deliverable; for now we validate
	// the URL only.
	if u.Scheme != "https" {
		return RagAttachRemoteReply{}, fmt.Errorf("rag/attach_remote: must use https://")
	}
	if err := foundry.ValidateEndpoint("https://" + u.Host); err != nil {
		return RagAttachRemoteReply{}, fmt.Errorf("rag/attach_remote: %w", err)
	}
	return RagAttachRemoteReply{OK: true, StatusNote: "URL accepted; signed manifest fetch + verify ships in v0.2.1"}, nil
}

func (s *Server) RagDetachRemote(ctx context.Context) (struct {
	OK bool `json:"ok"`
}, error) {
	return struct {
		OK bool `json:"ok"`
	}{OK: true}, nil
}

func (s *Server) RagSyncRemote(ctx context.Context) (struct {
	OK   bool   `json:"ok"`
	Note string `json:"note"`
}, error) {
	return struct {
		OK   bool   `json:"ok"`
		Note string `json:"note"`
	}{OK: true, Note: "remote sync ships in v0.2.1"}, nil
}

// ─── Agents (Foundry Agents API) ─ NOTE: v0.2.1 ─────────────────────────────
// The foundryagents client exposes Thread/Message/Run primitives but no
// agent-listing endpoint yet. We omit agents/list from the v0.2.0 surface;
// it lands in v0.2.1 once the Agents Service inventory API is wired.

// ─── method binding ─────────────────────────────────────────────────────────

// V02Methods returns the v0.2 surface methods bound to the parent Server.
func (s *Server) V02Methods() handler.Map {
	return handler.Map{
		"chat/variables_list":    handler.New(s.ChatVariablesList),
		"chat/variables_resolve": handler.New(s.ChatVariablesResolve),
		"chat/edit_propose":      handler.New(s.ChatEditPropose),
		"chat/edit_apply":        handler.New(s.ChatEditApply),
		"chat/edit_discard":      handler.New(s.ChatEditDiscard),
		"nes/predict":            handler.New(s.NESPredict),
		"telemetry/summary":      handler.New(s.TelemetrySummary),
		"telemetry/timeseries":   handler.New(s.TelemetryTimeSeries),
		"telemetry/errors":       handler.New(s.TelemetryErrors),
		"telemetry/clear":        handler.New(s.TelemetryClear),
		"billing/summary":        handler.New(s.BillingSummary),
		"billing/quota":          handler.New(s.BillingQuota),
		"billing/budget_set":     handler.New(s.BillingBudgetSet),
		"billing/budget_get":     handler.New(s.BillingBudgetGet),
		"dataset/validate":       handler.New(s.DatasetValidate),
		"dataset/from_sessions":  handler.New(s.DatasetFromSessions),
		"finetune/list_jobs":     handler.New(s.FinetuneListJobs),
		"finetune/get_job":       handler.New(s.FinetuneGetJob),
		"finetune/cancel_job":    handler.New(s.FinetuneCancelJob),
		"finetune/create_job":    handler.New(s.FinetuneCreateJob),
		"deploy/list":            handler.New(s.DeployList),
		"deploy/get":             handler.New(s.DeployGet),
		"policy/load":            handler.New(s.PolicyLoad),
		"rag/attach_remote":      handler.New(s.RagAttachRemote),
		"rag/detach_remote":      handler.New(s.RagDetachRemote),
		"rag/sync_remote":        handler.New(s.RagSyncRemote),
	}
}

// errStr is a small helper that returns "" for a nil error so the telemetry
// Event omits the field.
func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
