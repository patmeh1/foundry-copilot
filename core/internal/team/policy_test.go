package team

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicy_Missing(t *testing.T) {
	p, err := LoadPolicy(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.AllowedDeployments) != 0 || len(p.AllowedTools) != 0 {
		t.Fatalf("expected empty policy, got %+v", p)
	}
}

func TestLoadPolicy_Parses(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"allowed_deployments":["gpt-4o","gpt-4o-mini"],"allowed_tools":["read_file"],"max_tokens":40000}`
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.AllowedDeployments) != 2 || p.AllowedDeployments[0] != "gpt-4o" {
		t.Fatalf("deployments mismatch: %+v", p.AllowedDeployments)
	}
	if p.MaxTokens != 40000 {
		t.Fatalf("max_tokens=%d", p.MaxTokens)
	}
}

func TestCheckDeployment_EmptyAllows(t *testing.T) {
	if err := (Policy{}).CheckDeployment("any-thing"); err != nil {
		t.Fatalf("empty policy must allow all: %v", err)
	}
}

func TestCheckDeployment_DisallowedFails(t *testing.T) {
	p := Policy{AllowedDeployments: []string{"a"}}
	err := p.CheckDeployment("b")
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("want ErrPolicyViolation, got %v", err)
	}
}

func TestCheckTool_DisallowedFails(t *testing.T) {
	p := Policy{AllowedTools: []string{"read_file"}}
	err := p.CheckTool("shell")
	if !errors.Is(err, ErrPolicyViolation) {
		t.Fatalf("want ErrPolicyViolation, got %v", err)
	}
}
