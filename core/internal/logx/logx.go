// Package logx wraps log/slog with a sidecar-safe default:
//   - logs go to STDERR only (stdout is reserved for JSON-RPC framing)
//   - structured JSON in production, text in TTY
package logx

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a *slog.Logger that writes structured JSON to stderr.
// level may be "debug", "info", "warn", or "error" (case-insensitive).
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h).With("svc", "foundry-copilot-sidecar")
}
