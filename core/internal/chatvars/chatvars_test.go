package chatvars

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltins_HasEightEntries(t *testing.T) {
	if got := len(Builtins); got != 8 {
		t.Fatalf("Builtins must have 8 entries, got %d", got)
	}
	want := map[string]bool{
		"#file": true, "#selection": true, "#editor": true,
		"#terminalLastCommand": true, "#codebase": true, "#problems": true,
		"#folder": true, "#symbol": true,
	}
	for _, v := range Builtins {
		if !want[v.Name] {
			t.Errorf("unexpected builtin %q", v.Name)
		}
		delete(want, v.Name)
	}
	for missing := range want {
		t.Errorf("missing builtin %q", missing)
	}
}

func TestResolve_File_ReadsContent(t *testing.T) {
	root := t.TempDir()
	rel := "hello.txt"
	abs := filepath.Join(root, rel)
	if err := os.WriteFile(abs, []byte("hi there"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Resolve(context.Background(), ResolveRequest{Name: "#file", Arg: rel, WorkspaceRoot: root}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "hi there" {
		t.Fatalf("content mismatch: %q", out.Content)
	}
}

func TestResolve_File_RejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#file", Arg: "../etc/passwd", WorkspaceRoot: root}, nil)
	if err == nil || !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}
}

func TestResolve_File_RejectsAbsolutePath(t *testing.T) {
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#file", Arg: "/etc/passwd", WorkspaceRoot: "/tmp"}, nil)
	if err == nil || !errors.Is(err, ErrPathEscape) {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}
}

func TestResolve_File_TooLargeTruncated(t *testing.T) {
	root := t.TempDir()
	rel := "big.bin"
	abs := filepath.Join(root, rel)
	if err := os.WriteFile(abs, make([]byte, MaxFileBytes+100), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Resolve(context.Background(), ResolveRequest{Name: "#file", Arg: rel, WorkspaceRoot: root}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "[foundry-copilot: file truncated") {
		t.Fatalf("expected truncation marker; got: %q", out.Content[len(out.Content)-100:])
	}
}

func TestResolve_Selection_Echo(t *testing.T) {
	out, err := Resolve(context.Background(), ResolveRequest{Name: "#selection", Selection: "abc"}, nil)
	if err != nil || out.Content != "abc" {
		t.Fatalf("got %q err %v", out.Content, err)
	}
}

func TestResolve_Editor_Echo(t *testing.T) {
	out, _ := Resolve(context.Background(), ResolveRequest{Name: "#editor", EditorContent: "x"}, nil)
	if out.Content != "x" {
		t.Fatalf("got %q", out.Content)
	}
}

func TestResolve_TerminalLastCommand_Echo(t *testing.T) {
	out, _ := Resolve(context.Background(), ResolveRequest{Name: "#terminalLastCommand", LastTerminalCommand: "ls -la"}, nil)
	if out.Content != "ls -la" {
		t.Fatalf("got %q", out.Content)
	}
}

func TestResolve_Problems_Joins(t *testing.T) {
	out, _ := Resolve(context.Background(), ResolveRequest{Name: "#problems", Problems: []string{"e1", "e2"}}, nil)
	if out.Content != "e1\ne2" {
		t.Fatalf("got %q", out.Content)
	}
}

type fakeRag struct {
	gotQuery string
	gotK     int
	out      string
	err      error
}

func (f *fakeRag) Search(ctx context.Context, q string, k int) (string, error) {
	f.gotQuery = q
	f.gotK = k
	return f.out, f.err
}

func TestResolve_Codebase_RequiresSearcher(t *testing.T) {
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#codebase", Arg: "foo"}, nil)
	if !errors.Is(err, ErrNoSearcher) {
		t.Fatalf("expected ErrNoSearcher, got %v", err)
	}
}

func TestResolve_Codebase_RequiresArg(t *testing.T) {
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#codebase"}, &fakeRag{})
	if !errors.Is(err, ErrMissingArg) {
		t.Fatalf("expected ErrMissingArg, got %v", err)
	}
}

func TestResolve_Codebase_CallsSearcher(t *testing.T) {
	r := &fakeRag{out: "matched snippets"}
	out, err := Resolve(context.Background(), ResolveRequest{Name: "#codebase", Arg: "user auth"}, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Content != "matched snippets" {
		t.Fatalf("got %q", out.Content)
	}
	if r.gotQuery != "user auth" || r.gotK != 6 {
		t.Fatalf("searcher got query=%q k=%d", r.gotQuery, r.gotK)
	}
}

func TestResolve_Folder_ListsEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := Resolve(context.Background(), ResolveRequest{Name: "#folder", Arg: ".", WorkspaceRoot: root}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Content, "a.txt") || !strings.Contains(out.Content, "b/") {
		t.Fatalf("listing missing entries: %q", out.Content)
	}
}

func TestResolve_Symbol_NotImplemented(t *testing.T) {
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#symbol", Arg: "Foo"}, nil)
	if err == nil || !strings.Contains(err.Error(), "extension-side") {
		t.Fatalf("expected extension-side error, got %v", err)
	}
}

func TestResolve_UnknownVar_Error(t *testing.T) {
	_, err := Resolve(context.Background(), ResolveRequest{Name: "#nope"}, nil)
	if !errors.Is(err, ErrUnknownVariable) {
		t.Fatalf("expected ErrUnknownVariable, got %v", err)
	}
}
