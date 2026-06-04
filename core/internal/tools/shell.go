package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// Shell runs a single command via the platform's default shell. Disabled
// unless Allow is true. Output is captured (stdout+stderr merged) and
// truncated to keep token usage sane.
type Shell struct {
	Root    string
	Allow   bool
	Timeout time.Duration
}

const shellOutMax = 16 * 1024

func (Shell) Name() string { return "shell" }
func (Shell) Description() string {
	return "Run a shell command in the workspace and return its combined stdout+stderr (truncated). Use sparingly. Requires the user to have enabled shell access."
}
func (Shell) ParametersSchema() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"command"},
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "the command line to run"},
		},
		"additionalProperties": false,
	}
}
func (t Shell) Invoke(ctx context.Context, argsJSON string) (string, error) {
	if !t.Allow {
		return "", fmt.Errorf("shell disabled (set foundryCopilot.agent.allowShell=true to enable)")
	}
	var a struct {
		Command string `json:"command"`
	}
	if err := decodeArgs(argsJSON, &a); err != nil {
		return "", err
	}
	if a.Command == "" {
		return "", fmt.Errorf("shell: missing 'command'")
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(cctx, "cmd.exe", "/C", a.Command)
	} else {
		cmd = exec.CommandContext(cctx, "sh", "-c", a.Command)
	}
	if t.Root != "" {
		cmd.Dir = t.Root
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := buf.Bytes()
	if len(out) > shellOutMax {
		out = append(out[:shellOutMax], []byte("\n…[truncated]…")...)
	}
	if runErr != nil {
		return string(out), fmt.Errorf("shell: %w", runErr)
	}
	return string(out), nil
}
