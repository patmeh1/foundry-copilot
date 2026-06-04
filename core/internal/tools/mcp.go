package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/patmeh1/foundry-copilot/core/internal/mcpx"
)

// MCPTool exposes one tool discovered on a connected MCP server to the
// agent loop. The exposed name is namespaced as "mcp__{server}__{name}"
// to avoid collisions with the built-in tool set.
type MCPTool struct {
	Manager  *mcpx.Manager
	ServerID string
	ToolName string
	Desc     string
	Schema   map[string]any
}

func (t MCPTool) Name() string { return "mcp__" + t.ServerID + "__" + t.ToolName }
func (t MCPTool) Description() string {
	if t.Desc != "" {
		return fmt.Sprintf("[%s] %s", t.ServerID, t.Desc)
	}
	return fmt.Sprintf("[%s] tool %s", t.ServerID, t.ToolName)
}
func (t MCPTool) ParametersSchema() map[string]any {
	if t.Schema == nil {
		return map[string]any{"type": "object"}
	}
	return t.Schema
}

func (t MCPTool) Invoke(ctx context.Context, argsJSON string) (string, error) {
	if t.Manager == nil {
		return "", fmt.Errorf("%s: mcp manager not set", t.Name())
	}
	var args map[string]any
	if argsJSON == "" {
		args = map[string]any{}
	} else if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("%s: bad args: %w", t.Name(), err)
	}
	return t.Manager.CallTool(ctx, t.ServerID, t.ToolName, args)
}
