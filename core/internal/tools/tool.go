// Package tools defines the Tool interface, a Registry, and the default
// set of tools the agent loop can call. Tools are deliberately small,
// composable units; each is scoped to a workspace root and produces a
// string result the model can read.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// Tool is the interface every agent tool implements.
type Tool interface {
	Name() string
	Description() string
	// ParametersSchema returns a JSON Schema (object) describing the args.
	ParametersSchema() map[string]any
	// Invoke runs the tool. argsJSON is the raw JSON string the model
	// produced. Implementations MUST be deterministic w.r.t. arguments
	// and tolerate cancellation via ctx.
	Invoke(ctx context.Context, argsJSON string) (string, error)
}

// Registry is an ordered set of tools indexed by name.
type Registry struct {
	order []string
	byName map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{byName: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	if _, exists := r.byName[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.byName[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// All returns the tools in registration order.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.byName[n])
	}
	return out
}

// Len reports how many tools are registered.
func (r *Registry) Len() int { return len(r.order) }

// ─── helpers shared by tool implementations ────────────────────────────────

// safeJoin joins root and rel, ensuring the result stays under root.
// Rejects absolute rel paths and any ".." that escapes the root.
func safeJoin(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("tools: workspace root not configured")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("tools: absolute path not allowed: %s", rel)
	}
	clean := filepath.Clean(rel)
	if strings.HasPrefix(clean, "..") || clean == ".." {
		return "", fmt.Errorf("tools: path escapes workspace: %s", rel)
	}
	full := filepath.Join(root, clean)
	// Final defence: resolved path must still live under root.
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(fullAbs, rootAbs) {
		return "", fmt.Errorf("tools: path escapes workspace: %s", rel)
	}
	return full, nil
}

// decodeArgs is a tiny convenience wrapper around json.Unmarshal.
func decodeArgs(raw string, out any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}
