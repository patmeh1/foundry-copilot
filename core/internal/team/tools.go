// tools.go loads .foundry/tools/*.json overlay descriptors. Each descriptor
// describes one read-only HTTP tool the assistant may invoke. v0.2.0 only
// parses + validates the manifest shape; v0.2.1 wires the runtime dispatcher.
package team

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ToolOverlay is one overlay tool entry from disk.
type ToolOverlay struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	URLTemplate string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	JQFilter    string            `json:"jq,omitempty"`
}

// LoadOverlays reads every .json file under .foundry/tools/. Missing dir
// returns nil + nil (no overlays). A file with empty Name or URLTemplate is
// rejected with an actionable error so users see the bad file path.
func LoadOverlays(workspaceRoot string) ([]ToolOverlay, error) {
	dir := filepath.Join(workspaceRoot, ".foundry", "tools")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("team: read tools dir: %w", err)
	}
	var out []ToolOverlay
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("team: read %s: %w", full, err)
		}
		var t ToolOverlay
		if err := json.Unmarshal(b, &t); err != nil {
			return nil, fmt.Errorf("team: parse %s: %w", full, err)
		}
		if t.Name == "" {
			return nil, fmt.Errorf("team: %s: 'name' is required", full)
		}
		if t.URLTemplate == "" {
			return nil, fmt.Errorf("team: %s: 'url' is required", full)
		}
		out = append(out, t)
	}
	return out, nil
}
