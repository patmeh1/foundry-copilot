package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestChunkFile_WindowsAndOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	var content string
	for i := 1; i <= 200; i++ {
		content += "line " + itoa(i) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	chunks, err := ChunkFile(path, "a.go", ChunkOptions{WindowLines: 60, OverlapLines: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 3 {
		t.Fatalf("expected >= 3 chunks, got %d", len(chunks))
	}
	if chunks[0].StartLine != 1 || chunks[0].EndLine != 60 {
		t.Fatalf("chunk[0] range = %d-%d", chunks[0].StartLine, chunks[0].EndLine)
	}
	if chunks[1].StartLine != 51 {
		t.Fatalf("expected overlap, chunk[1] starts at %d", chunks[1].StartLine)
	}
}

func TestStore_PersistAndSearch(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "store.gob"))
	if err != nil {
		t.Fatal(err)
	}
	chunks := []Chunk{
		{ID: "a", Path: "a", Text: "alpha", Vec: []float32{1, 0, 0}},
		{ID: "b", Path: "b", Text: "beta", Vec: []float32{0, 1, 0}},
		{ID: "c", Path: "c", Text: "gamma", Vec: []float32{0.7071, 0.7071, 0}},
	}
	if err := s.Replace(chunks); err != nil {
		t.Fatal(err)
	}
	hits := s.Search([]float32{1, 0, 0}, 2)
	if len(hits) != 2 {
		t.Fatalf("got %d hits", len(hits))
	}
	if hits[0].Chunk.ID != "a" {
		t.Fatalf("top hit = %s", hits[0].Chunk.ID)
	}
	// Reload and search again.
	s2, err := Open(filepath.Join(dir, "store.gob"))
	if err != nil {
		t.Fatal(err)
	}
	if s2.Len() != 3 {
		t.Fatalf("reloaded len = %d", s2.Len())
	}
}

func TestWalkWorkspace_SkipsAndPicks(t *testing.T) {
	dir := t.TempDir()
	mustMkFile(t, filepath.Join(dir, "a.go"), "x")
	mustMkFile(t, filepath.Join(dir, "node_modules", "x.js"), "x")
	mustMkFile(t, filepath.Join(dir, "sub", "b.py"), "x")
	mustMkFile(t, filepath.Join(dir, "binary.bin"), "x")
	files, err := WalkWorkspace(context.Background(), dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	has := func(s string) bool {
		for _, f := range files {
			if f == s {
				return true
			}
		}
		return false
	}
	if !has("a.go") || !has(filepath.Join("sub", "b.py")) {
		t.Fatalf("missing expected files: %v", files)
	}
	for _, f := range files {
		if f == filepath.Join("node_modules", "x.js") {
			t.Fatalf("should have skipped node_modules: %v", files)
		}
		if f == "binary.bin" {
			t.Fatalf("should have skipped .bin: %v", files)
		}
	}
}

func mustMkFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tiny local itoa to avoid pulling strconv into the test only.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
