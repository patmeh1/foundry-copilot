// Package mcpx wires Model Context Protocol (MCP) stdio clients into
// the sidecar. Each connection runs a child process and is identified
// by a stable user-chosen id. Tools discovered on connect are exposed
// to the agent loop via the tools.MCPTool adapter.
//
// Named `mcpx` (not `mcp`) to avoid colliding with the upstream
// `github.com/mark3labs/mcp-go/mcp` import.
package mcpx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcp "github.com/mark3labs/mcp-go/mcp"
)

// ServerSpec describes how to launch a stdio MCP server.
type ServerSpec struct {
	ID      string            `json:"id"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// ToolInfo is the manager-level view of a discovered MCP tool.
type ToolInfo struct {
	ServerID    string
	Name        string
	Description string
	InputSchema map[string]any
}

type connection struct {
	spec   ServerSpec
	client *mcpclient.Client
}

// Manager owns the set of live MCP connections. Safe for concurrent use.
type Manager struct {
	mu    sync.RWMutex
	conns map[string]*connection
}

func New() *Manager {
	return &Manager{conns: map[string]*connection{}}
}

// Connect launches the given MCP server and performs the initialize
// handshake. Replacing an existing id closes the old connection first.
func (m *Manager) Connect(ctx context.Context, spec ServerSpec) error {
	if strings.TrimSpace(spec.ID) == "" {
		return errors.New("mcp: id required")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return errors.New("mcp: command required")
	}
	// Render env as KEY=VALUE strings expected by NewStdioMCPClient.
	envSlice := make([]string, 0, len(spec.Env))
	for k, v := range spec.Env {
		envSlice = append(envSlice, k+"="+v)
	}
	c, err := mcpclient.NewStdioMCPClient(spec.Command, envSlice, spec.Args...)
	if err != nil {
		return fmt.Errorf("mcp: start %q: %w", spec.ID, err)
	}
	initCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := c.Initialize(initCtx, mcp.InitializeRequest{}); err != nil {
		_ = c.Close()
		return fmt.Errorf("mcp: initialize %q: %w", spec.ID, err)
	}
	m.mu.Lock()
	if old, ok := m.conns[spec.ID]; ok {
		_ = old.client.Close()
	}
	m.conns[spec.ID] = &connection{spec: spec, client: c}
	m.mu.Unlock()
	return nil
}

// Disconnect closes the named connection. Unknown ids return nil.
func (m *Manager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[id]
	if !ok {
		return nil
	}
	delete(m.conns, id)
	return c.client.Close()
}

// List returns the ids of currently connected servers.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.conns))
	for id := range m.conns {
		out = append(out, id)
	}
	return out
}

// ListTools returns the tools advertised by a single connected server.
func (m *Manager) ListTools(ctx context.Context, id string) ([]ToolInfo, error) {
	m.mu.RLock()
	c, ok := m.conns[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mcp: unknown server %q", id)
	}
	resp, err := c.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	out := make([]ToolInfo, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		// ToolInputSchema marshals to a JSON Schema object; round-trip
		// via JSON to get a plain map[string]any for the agent loop.
		schema := map[string]any{"type": "object"}
		if raw, err := t.InputSchema.MarshalJSON(); err == nil {
			var m map[string]any
			if jsonUnmarshal(raw, &m) == nil && len(m) > 0 {
				schema = m
			}
		}
		out = append(out, ToolInfo{
			ServerID: id, Name: t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return out, nil
}

// ListAllTools returns tools across every connected server.
func (m *Manager) ListAllTools(ctx context.Context) []ToolInfo {
	var out []ToolInfo
	for _, id := range m.List() {
		tools, err := m.ListTools(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, tools...)
	}
	return out
}

// CallTool invokes the named tool on a server with the supplied raw
// argument map. The returned string concatenates all text content
// returned by the tool; non-text content is summarised as `[<type>]`.
func (m *Manager) CallTool(ctx context.Context, serverID, name string, args map[string]any) (string, error) {
	m.mu.RLock()
	c, ok := m.conns[serverID]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("mcp: unknown server %q", serverID)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	resp, err := c.client.CallTool(ctx, req)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, part := range resp.Content {
		if tc, ok := mcp.AsTextContent(part); ok {
			b.WriteString(tc.Text)
			b.WriteByte('\n')
		} else {
			fmt.Fprintf(&b, "[non-text content]\n")
		}
	}
	if resp.IsError {
		return b.String(), fmt.Errorf("mcp tool error")
	}
	return b.String(), nil
}

// Close shuts down all connections.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conns {
		_ = c.client.Close()
		delete(m.conns, id)
	}
}
