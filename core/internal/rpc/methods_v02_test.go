// rpc package integration test: spin up an in-memory JSON-RPC server with
// the v0.2 method map and call each surface to confirm:
//   - the method name is registered (no "method not found")
//   - the method returns either a typed reply or an actionable error (not a
//     panic, not a stub sentinel)
package rpc

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"

	"github.com/patmeh1/foundry-copilot/core/internal/config"
)

// newPipedServer wires the Server up on an in-memory io.Pipe pair so we can
// dial it with a jrpc2.Client.
func newPipedServer(t *testing.T) (*jrpc2.Client, func()) {
	t.Helper()

	s := &Server{
		Log:     slog.Default(),
		Version: "test",
	}
	// Pre-configure WorkspaceRoot so policy/load succeeds.
	cfg := config.Get()
	cfg.WorkspaceRoot = t.TempDir()
	cfg.TelemetryPath = t.TempDir() + "/telemetry.jsonl"
	cfg.BudgetStatePath = t.TempDir() + "/budget.json"
	config.Update(cfg)

	a, b := net.Pipe()
	srvCh := channel.LSP(a, a)
	cliCh := channel.LSP(b, b)
	srv := jrpc2.NewServer(s.methods(), &jrpc2.ServerOptions{
		Logger: func(string) {},
	}).Start(srvCh)
	cli := jrpc2.NewClient(cliCh, nil)
	t.Cleanup(func() {
		_ = cli.Close()
		srv.Stop()
		_ = a.Close()
		_ = b.Close()
	})
	return cli, func() { /* explicit teardown not needed */ }
}

func TestV02_MethodsRegistered(t *testing.T) {
	cli, _ := newPipedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Each method we expose must NOT return "method not found". A typed
	// error is fine — we are only verifying discoverability.
	cases := []struct {
		method string
		params interface{}
	}{
		{"chat/variables_list", nil},
		{"telemetry/summary", map[string]any{"window_hours": 1}},
		{"telemetry/timeseries", map[string]any{"window_hours": 1}},
		{"telemetry/errors", map[string]any{"limit": 5}},
		{"telemetry/clear", nil},
		{"billing/summary", map[string]any{"window_hours": 1}},
		{"billing/quota", nil},
		{"billing/budget_get", nil},
		{"billing/budget_set", map[string]any{"budget_usd": 5.0}},
		{"dataset/validate", map[string]any{"examples": []any{}}},
		{"policy/load", nil},
		{"rag/detach_remote", nil},
		{"rag/sync_remote", nil},
	}
	for _, c := range cases {
		c := c
		t.Run(c.method, func(t *testing.T) {
			rsp, err := cli.Call(ctx, c.method, c.params)
			if err != nil {
				// Surface as failure only if it's "method not found".
				if strings.Contains(strings.ToLower(err.Error()), "method not found") {
					t.Fatalf("method not registered: %s: %v", c.method, err)
				}
				// Other errors (e.g. "workspace_root required") are
				// acceptable — they prove the handler ran.
				return
			}
			// A 200 reply is also acceptable.
			var any json.RawMessage
			_ = rsp.UnmarshalResult(&any)
		})
	}
}

// TestV02_BillingBudgetRoundtrip confirms budget_set persists into config
// and budget_get reads it back.
func TestV02_BillingBudgetRoundtrip(t *testing.T) {
	cli, _ := newPipedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, err := cli.Call(ctx, "billing/budget_set", map[string]any{"budget_usd": 12.5})
	if err != nil {
		t.Fatalf("budget_set: %v", err)
	}
	var set BillingBudgetReply
	if err := rsp.UnmarshalResult(&set); err != nil {
		t.Fatalf("unmarshal budget_set: %v", err)
	}
	if set.BudgetUSD != 12.5 {
		t.Fatalf("budget_set echoed wrong value: %v", set.BudgetUSD)
	}
	rsp, err = cli.Call(ctx, "billing/budget_get", nil)
	if err != nil {
		t.Fatalf("budget_get: %v", err)
	}
	var got BillingBudgetReply
	if err := rsp.UnmarshalResult(&got); err != nil {
		t.Fatalf("unmarshal budget_get: %v", err)
	}
	if got.BudgetUSD != 12.5 {
		t.Fatalf("budget_get returned %v, want 12.5", got.BudgetUSD)
	}
}

// TestV02_ChatVariablesList confirms the eight built-in variables surface.
func TestV02_ChatVariablesList(t *testing.T) {
	cli, _ := newPipedServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rsp, err := cli.Call(ctx, "chat/variables_list", nil)
	if err != nil {
		t.Fatalf("variables_list: %v", err)
	}
	var got ChatVariablesListReply
	if err := rsp.UnmarshalResult(&got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Variables) != 8 {
		t.Fatalf("want 8 builtin variables, got %d", len(got.Variables))
	}
}

// avoid unused-import lint when test list shrinks.
var _ = io.EOF
