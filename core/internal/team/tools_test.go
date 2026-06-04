package team

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOverlays_Missing(t *testing.T) {
	out, err := LoadOverlays(t.TempDir())
	if err != nil || out != nil {
		t.Fatalf("expected nil overlays, got out=%v err=%v", out, err)
	}
}

func TestLoadOverlays_Parses(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry", "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"name":"jira","description":"Read JIRA ticket","url":"https://acme.atlassian.net/rest/api/3/issue/{arg}","headers":{"Authorization":"Bearer X"}}`
	if err := os.WriteFile(filepath.Join(dir, "jira.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := LoadOverlays(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "jira" {
		t.Fatalf("overlay mismatch: %+v", out)
	}
}

func TestLoadOverlays_RejectsMissingName(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry", "tools")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"url":"https://x"}`), 0o644)
	_, err := LoadOverlays(root)
	if err == nil || !strings.Contains(err.Error(), "'name'") {
		t.Fatalf("want name error, got %v", err)
	}
}

func TestLoadOverlays_RejectsMissingURL(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".foundry", "tools")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"name":"x"}`), 0o644)
	_, err := LoadOverlays(root)
	if err == nil || !strings.Contains(err.Error(), "'url'") {
		t.Fatalf("want url error, got %v", err)
	}
}
