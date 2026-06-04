package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fsReadMax = 64 * 1024 // 64 KiB cap to keep tokens sane

// FSRead reads a UTF-8 text file from the workspace.
type FSRead struct{ Root string }

func (FSRead) Name() string { return "fs_read" }
func (FSRead) Description() string {
	return "Read a UTF-8 text file from the workspace. Returns the file contents (truncated to ~64 KiB). The path argument is workspace-relative."
}
func (FSRead) ParametersSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path"},
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "workspace-relative file path"},
		},
		"additionalProperties": false,
	}
}
func (t FSRead) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := decodeArgs(argsJSON, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", fmt.Errorf("fs_read: missing 'path'")
	}
	full, err := safeJoin(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if len(b) > fsReadMax {
		return string(b[:fsReadMax]) + "\n\n…[truncated]…", nil
	}
	return string(b), nil
}

// FSWrite creates or overwrites a file. Only enabled when Allow is true.
type FSWrite struct {
	Root  string
	Allow bool
}

func (FSWrite) Name() string { return "fs_write" }
func (FSWrite) Description() string {
	return "Create or overwrite a UTF-8 text file in the workspace. The path is workspace-relative. Parent directories are created as needed. Requires the user to have enabled write access."
}
func (FSWrite) ParametersSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"path", "content"},
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "workspace-relative file path"},
			"content": map[string]any{"type": "string", "description": "the full file contents to write"},
		},
		"additionalProperties": false,
	}
}
func (t FSWrite) Invoke(ctx context.Context, argsJSON string) (string, error) {
	if !t.Allow {
		return "", fmt.Errorf("fs_write disabled (set foundryCopilot.agent.allowWrite=true to enable)")
	}
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decodeArgs(argsJSON, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", fmt.Errorf("fs_write: missing 'path'")
	}
	full, err := safeJoin(t.Root, a.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), strings.TrimPrefix(full, t.Root+string(filepath.Separator))), nil
}
