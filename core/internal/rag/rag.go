// Package rag is the workspace indexing + retrieval layer used by the
// /agent loop's rag_search tool and the index/* RPC methods.
//
// Storage is a flat slice of chunks persisted as gob on disk; for the
// MVP (workspaces of up to ~thousands of small files) this is fast,
// dependency-free, and easy to reason about. The store is loaded lazily
// and saved atomically (write to temp + rename).
package rag

import (
	"bufio"
	"context"
	"encoding/gob"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Chunk is one indexed segment of a file.
type Chunk struct {
	ID        string
	Path      string
	StartLine int
	EndLine   int
	Text      string
	Vec       []float32
}

// Store is an in-memory vector store with cosine similarity search.
type Store struct {
	mu     sync.RWMutex
	chunks []Chunk
	path   string
}

// Open loads (or creates) the store at the given path.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	defer f.Close()
	if err := gob.NewDecoder(f).Decode(&s.chunks); err != nil {
		// Treat a corrupt store as empty rather than failing the sidecar.
		s.chunks = nil
	}
	return s, nil
}

// Len returns the number of chunks held.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.chunks)
}

// Replace atomically swaps the entire contents of the store and persists.
func (s *Store) Replace(chunks []Chunk) error {
	s.mu.Lock()
	s.chunks = chunks
	s.mu.Unlock()
	return s.Persist()
}

// Persist writes the store to disk atomically.
func (s *Store) Persist() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := gob.NewEncoder(f).Encode(s.chunks); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, s.path)
}

// Hit is one search result.
type Hit struct {
	Chunk Chunk
	Score float32
}

// Search returns the top-k chunks by cosine similarity to query.
func (s *Store) Search(query []float32, k int) []Hit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.chunks) == 0 || k <= 0 {
		return nil
	}
	qn := norm(query)
	if qn == 0 {
		return nil
	}
	scored := make([]Hit, 0, len(s.chunks))
	for _, c := range s.chunks {
		s := cosine(query, c.Vec, qn)
		scored = append(scored, Hit{Chunk: c, Score: s})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > k {
		scored = scored[:k]
	}
	return scored
}

func norm(v []float32) float32 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return float32(math.Sqrt(s))
}

func cosine(a, b []float32, normA float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	var nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		nb += float64(b[i]) * float64(b[i])
	}
	nbf := float32(math.Sqrt(nb))
	if normA == 0 || nbf == 0 {
		return 0
	}
	return float32(dot) / (normA * nbf)
}

// ─── chunking ──────────────────────────────────────────────────────────────

// ChunkOptions controls the line-window chunker.
type ChunkOptions struct {
	WindowLines  int // default 60
	OverlapLines int // default 10
	MaxBytes     int // default 16 KiB per chunk
}

// ChunkFile splits a file into chunks. Returns one chunk per window.
func ChunkFile(path, rel string, opts ChunkOptions) ([]Chunk, error) {
	if opts.WindowLines <= 0 {
		opts.WindowLines = 60
	}
	if opts.OverlapLines < 0 {
		opts.OverlapLines = 10
	}
	if opts.OverlapLines >= opts.WindowLines {
		opts.OverlapLines = opts.WindowLines / 3
	}
	if opts.MaxBytes <= 0 {
		opts.MaxBytes = 16 * 1024
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, nil
	}
	step := opts.WindowLines - opts.OverlapLines
	if step <= 0 {
		step = opts.WindowLines
	}
	var out []Chunk
	for start := 0; start < len(lines); start += step {
		end := start + opts.WindowLines
		if end > len(lines) {
			end = len(lines)
		}
		text := strings.Join(lines[start:end], "\n")
		if len(text) > opts.MaxBytes {
			text = text[:opts.MaxBytes]
		}
		out = append(out, Chunk{
			ID:        fmt.Sprintf("%s#L%d-L%d", rel, start+1, end),
			Path:      rel,
			StartLine: start + 1,
			EndLine:   end,
			Text:      text,
		})
		if end == len(lines) {
			break
		}
	}
	return out, nil
}

// ─── workspace walk ────────────────────────────────────────────────────────

// Indexable extensions — keep the surface narrow; binary files and noisy
// directories are skipped.
var indexExt = map[string]struct{}{
	".go": {}, ".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".py": {},
	".rs": {}, ".java": {}, ".kt": {}, ".swift": {}, ".c": {}, ".h": {},
	".cc": {}, ".cpp": {}, ".hpp": {}, ".rb": {}, ".php": {}, ".cs": {},
	".scala": {}, ".sh": {}, ".bash": {}, ".zsh": {}, ".lua": {}, ".sql": {},
	".md": {}, ".mdx": {}, ".rst": {}, ".txt": {}, ".yaml": {}, ".yml": {},
	".json": {}, ".toml": {}, ".ini": {}, ".html": {}, ".css": {}, ".scss": {},
	".vue": {}, ".svelte": {},
}

var skipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, ".venv": {}, "venv": {},
	"dist": {}, "build": {}, "target": {}, "out": {},
	".next": {}, ".cache": {}, ".turbo": {}, ".gradle": {},
	"__pycache__": {}, ".idea": {}, ".vscode": {},
}

// WalkWorkspace walks root and returns all index-eligible relative paths.
func WalkWorkspace(ctx context.Context, root string, maxFiles int) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if _, ok := indexExt[ext]; !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 1*1024*1024 {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, rel)
		if maxFiles > 0 && len(out) >= maxFiles {
			return filepath.SkipAll
		}
		return nil
	})
	return out, err
}
