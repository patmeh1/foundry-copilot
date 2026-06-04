package tools

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CodeSearch performs a recursive substring search across the workspace,
// honouring a small denylist of well-known noisy directories.
type CodeSearch struct{ Root string }

const (
	searchMaxFiles   = 4000
	searchMaxMatches = 200
	searchMaxBytes   = 1 * 1024 * 1024 // skip files larger than 1MB
)

var searchSkipDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, ".venv": {}, "venv": {},
	"dist": {}, "build": {}, "target": {}, "out": {},
	".next": {}, ".cache": {}, ".turbo": {}, ".gradle": {},
	"__pycache__": {}, ".idea": {}, ".vscode": {},
}

func (CodeSearch) Name() string { return "code_search" }
func (CodeSearch) Description() string {
	return "Search the workspace for a substring (case-sensitive). Returns up to 200 matches as 'path:line: text'. Optionally restrict to files matching a glob pattern."
}
func (CodeSearch) ParametersSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"query"},
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "literal substring to search for"},
			"glob":  map[string]any{"type": "string", "description": "optional filename glob, e.g. '*.go'"},
		},
		"additionalProperties": false,
	}
}
func (t CodeSearch) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Query string `json:"query"`
		Glob  string `json:"glob"`
	}
	if err := decodeArgs(argsJSON, &a); err != nil {
		return "", err
	}
	if a.Query == "" {
		return "", fmt.Errorf("code_search: missing 'query'")
	}
	if t.Root == "" {
		return "", fmt.Errorf("code_search: workspace root not configured")
	}
	var (
		matches      []string
		filesScanned int
	)
	queryBytes := []byte(a.Query)
	err := filepath.WalkDir(t.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries silently
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if _, skip := searchSkipDirs[d.Name()]; skip && path != t.Root {
				return filepath.SkipDir
			}
			return nil
		}
		if filesScanned >= searchMaxFiles || len(matches) >= searchMaxMatches {
			return filepath.SkipAll
		}
		if a.Glob != "" {
			ok, _ := filepath.Match(a.Glob, d.Name())
			if !ok {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > searchMaxBytes {
			return nil
		}
		filesScanned++
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 64*1024), 512*1024)
		rel, _ := filepath.Rel(t.Root, path)
		line := 0
		for scanner.Scan() {
			line++
			if containsBytes(scanner.Bytes(), queryBytes) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, line, strings.TrimSpace(scanner.Text())))
				if len(matches) >= searchMaxMatches {
					break
				}
			}
		}
		_ = f.Close()
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	return strings.Join(matches, "\n"), nil
}

func containsBytes(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if bytesEqual(haystack[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
