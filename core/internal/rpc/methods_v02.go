// methods_v02.go adds the v0.2 method bindings as ErrNotImplemented stubs.
// Each stub mirrors the JSON-RPC method name registered on the extension
// side so integration tests can confirm method discovery now and the real
// behavior is wired in v0.2.1.
//
// IMPORTANT: stubs MUST return a typed error (not panic) so the extension's
// withTimeout/showErrorMessage path renders a helpful message. The error
// text is intentionally identical across stubs to make filtering easy.
package rpc

import (
	"context"
	"errors"

	"github.com/creachadair/jrpc2/handler"
)

// ErrV02NotImplemented is the canonical sentinel for v0.2 method stubs.
var ErrV02NotImplemented = errors.New("foundry-copilot: this RPC method is a v0.2 scaffold; real handler lands in v0.2.1")

// V02StubMethods returns the handler map of v0.2 stubs. Combined with
// methods() at Run time.
func V02StubMethods() handler.Map {
	stub := func(_ context.Context) (any, error) { return nil, ErrV02NotImplemented }
	return handler.Map{
		"chat/edit_propose":      handler.New(stub),
		"chat/edit_apply":        handler.New(stub),
		"chat/edit_discard":      handler.New(stub),
		"chat/variables_list":    handler.New(stub),
		"chat/variables_resolve": handler.New(stub),
		"nes/predict":            handler.New(stub),
		"deploy/list":            handler.New(stub),
		"deploy/get":             handler.New(stub),
		"deploy/create":          handler.New(stub),
		"deploy/update":          handler.New(stub),
		"deploy/delete":          handler.New(stub),
		"deploy/test":            handler.New(stub),
		"telemetry/summary":      handler.New(stub),
		"telemetry/timeseries":   handler.New(stub),
		"telemetry/errors":       handler.New(stub),
		"telemetry/clear":        handler.New(stub),
		"dataset/from_sessions":  handler.New(stub),
		"dataset/upload":         handler.New(stub),
		"dataset/list":           handler.New(stub),
		"dataset/delete":         handler.New(stub),
		"finetune/create_job":    handler.New(stub),
		"finetune/get_job":       handler.New(stub),
		"finetune/list_jobs":     handler.New(stub),
		"finetune/cancel_job":    handler.New(stub),
		"policy/load":            handler.New(stub),
		"rag/attach_remote":      handler.New(stub),
		"rag/detach_remote":      handler.New(stub),
		"rag/sync_remote":        handler.New(stub),
		"billing/summary":        handler.New(stub),
		"billing/quota":          handler.New(stub),
		"billing/budget_set":     handler.New(stub),
		"billing/budget_get":     handler.New(stub),
	}
}
