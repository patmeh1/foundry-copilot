// Package team loads team-shared configuration that lives next to the code:
// .foundry/policy.json (deployment + tool allow-lists, max token budgets) and
// .foundry/tools/*.json (read-only HTTP tool overlays).
//
// v0.2.0 uses JSON (no new module dependencies). YAML support is planned for
// v0.2.1 — the on-disk filenames will gain a *.yaml fallback at that time.
package team

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Policy is the parsed .foundry/policy.json shape.
type Policy struct {
	AllowedDeployments []string `json:"allowed_deployments,omitempty"`
	AllowedTools       []string `json:"allowed_tools,omitempty"`
	MaxTokens          int      `json:"max_tokens,omitempty"`
}

// ErrPolicyViolation is the sentinel for any allow-list failure. RPC handlers
// must wrap this so the extension can render an actionable hint.
var ErrPolicyViolation = errors.New("policy_violation")

// LoadPolicy reads .foundry/policy.json. Missing file = zero-valued Policy +
// nil error (no policy means no restrictions).
func LoadPolicy(workspaceRoot string) (Policy, error) {
	path := filepath.Join(workspaceRoot, ".foundry", "policy.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Policy{}, nil
		}
		return Policy{}, fmt.Errorf("team: read policy: %w", err)
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("team: parse policy: %w", err)
	}
	return p, nil
}

// CheckDeployment returns ErrPolicyViolation if dep is not in AllowedDeployments
// (when the list is non-empty). Empty list = allow all (no policy).
func (p Policy) CheckDeployment(dep string) error {
	if len(p.AllowedDeployments) == 0 {
		return nil
	}
	for _, d := range p.AllowedDeployments {
		if d == dep {
			return nil
		}
	}
	return fmt.Errorf("%w: deployment %q is not in .foundry/policy.json allowed_deployments", ErrPolicyViolation, dep)
}

// CheckTool returns ErrPolicyViolation if name is not in AllowedTools.
func (p Policy) CheckTool(name string) error {
	if len(p.AllowedTools) == 0 {
		return nil
	}
	for _, n := range p.AllowedTools {
		if n == name {
			return nil
		}
	}
	return fmt.Errorf("%w: tool %q is not in .foundry/policy.json allowed_tools", ErrPolicyViolation, name)
}
