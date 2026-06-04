package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFSRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := FSRead{Root: dir}
	out, err := tool.Invoke(context.Background(), `{"path":"hello.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi there" {
		t.Fatalf("got %q", out)
	}
}

func TestFSRead_RejectsEscape(t *testing.T) {
	dir := t.TempDir()
	tool := FSRead{Root: dir}
	_, err := tool.Invoke(context.Background(), `{"path":"../etc/passwd"}`)
	if err == nil {
		t.Fatal("expected escape rejection")
	}
}

func TestFSWrite_GatedByAllow(t *testing.T) {
	dir := t.TempDir()
	tool := FSWrite{Root: dir, Allow: false}
	_, err := tool.Invoke(context.Background(), `{"path":"x.txt","content":"x"}`)
	if err == nil {
		t.Fatal("expected gating error")
	}
	tool.Allow = true
	if _, err := tool.Invoke(context.Background(), `{"path":"x.txt","content":"x"}`); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "x.txt"))
	if err != nil || string(b) != "x" {
		t.Fatalf("write failed: %v %q", err, b)
	}
}

func TestCodeSearch_FindsAndSkips(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.go"), "package a\nfunc Hello() {}\n")
	mustWrite(t, filepath.Join(dir, "node_modules", "skip.js"), "Hello world\n")
	tool := CodeSearch{Root: dir}
	out, err := tool.Invoke(context.Background(), `{"query":"Hello"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a.go:") {
		t.Fatalf("missing a.go match: %s", out)
	}
	if strings.Contains(out, "node_modules") {
		t.Fatalf("did not skip node_modules: %s", out)
	}
}

func TestShell_GatedAndRuns(t *testing.T) {
	dir := t.TempDir()
	tool := Shell{Root: dir, Allow: false, Timeout: 2 * time.Second}
	if _, err := tool.Invoke(context.Background(), `{"command":"echo hi"}`); err == nil {
		t.Fatal("expected gating error")
	}
	tool.Allow = true
	out, err := tool.Invoke(context.Background(), `{"command":"echo phase5"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "phase5") {
		t.Fatalf("missing echo output: %q", out)
	}
}

func TestRegistry_Order(t *testing.T) {
	r := NewRegistry()
	r.Register(FSRead{Root: "/tmp"})
	r.Register(FSWrite{Root: "/tmp"})
	r.Register(CodeSearch{Root: "/tmp"})
	names := []string{}
	for _, tt := range r.All() {
		names = append(names, tt.Name())
	}
	want := []string{"fs_read", "fs_write", "code_search"}
	for i, n := range want {
		if names[i] != n {
			t.Fatalf("order: got %v want %v", names, want)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
