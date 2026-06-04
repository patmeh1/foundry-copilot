// Package chatvars resolves the eight built-in chat variables that any
// surface (chat view, inline chat, quick chat, terminal chat) may splice
// into a prompt: #file, #selection, #editor, #terminalLastCommand,
// #codebase, #problems, #folder, #symbol.
//
// Resolution lives in the sidecar so all UI surfaces share one resolver
// (and one set of safety guards). The extension supplies context that only
// it has access to — selection text, last terminal command, diagnostics —
// via the ResolveRequest struct. File / folder / codebase resolution
// happens here against the workspace root.
//
// Safety: #file and #folder reject any path argument containing ".." or
// an absolute prefix. #file caps the returned content at MaxFileBytes.
package chatvars

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaxFileBytes is the largest content payload #file will return. Larger
// files are truncated and a note appended.
const MaxFileBytes = 256 * 1024

// MaxFolderEntries is the maximum number of entries #folder will return per
// directory. Beyond this, a single trailing line documents the cap.
const MaxFolderEntries = 1000

// ErrPathEscape is returned when an Arg attempts to escape the workspace root.
var ErrPathEscape = errors.New("chatvars: path escapes workspace root")

// ErrMissingArg is returned for variables that require an Arg when Arg is empty.
var ErrMissingArg = errors.New("chatvars: argument required for this variable")

// ErrUnknownVariable is returned when Name doesn't match a built-in.
var ErrUnknownVariable = errors.New("chatvars: unknown variable")

// ErrNoSearcher is returned for #codebase when the RAG store is not attached.
var ErrNoSearcher = errors.New("chatvars: no RAG index attached (run 'Foundry: Refresh RAG index' first)")

// Variable is the metadata exposed via chat/variables_list.
type Variable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TakesArg    bool   `json:"takes_arg"`
}

// Builtins is the canonical list of built-in variables.
var Builtins = []Variable{
	{"#file", "Insert the content of a workspace file", true},
	{"#selection", "Insert the user's current editor selection", false},
	{"#editor", "Insert the content of the active editor", false},
	{"#terminalLastCommand", "Insert the last command executed in the active terminal", false},
	{"#codebase", "Search the workspace RAG index for relevant snippets", true},
	{"#problems", "Insert current diagnostics for the active file", false},
	{"#folder", "Insert a listing of a folder", true},
	{"#symbol", "Insert the source of a code symbol", true},
}

// ResolveRequest is the parameter shape for chat/variables_resolve.
type ResolveRequest struct {
	Name                string   `json:"name"`
	Arg                 string   `json:"arg"`
	WorkspaceRoot       string   `json:"workspace_root"`
	Selection           string   `json:"selection,omitempty"`
	EditorPath          string   `json:"editor_path,omitempty"`
	EditorContent       string   `json:"editor_content,omitempty"`
	LastTerminalCommand string   `json:"last_terminal_command,omitempty"`
	Problems            []string `json:"problems,omitempty"`
}

// ResolveResponse is what the extension splices into the prompt history as
// a tool message.
type ResolveResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// CodebaseSearcher is the contract satisfied by core/internal/rag.Store.
// Wired in via the RPC server constructor.
type CodebaseSearcher interface {
	Search(ctx context.Context, query string, k int) (string, error)
}

// Resolve dispatches on req.Name. Returns a clear error for any unknown
// variable or unsafe argument.
func Resolve(ctx context.Context, req ResolveRequest, rag CodebaseSearcher) (ResolveResponse, error) {
	switch req.Name {
	case "#file":
		return resolveFile(req)
	case "#selection":
		return ResolveResponse{Name: req.Name, Content: req.Selection}, nil
	case "#editor":
		return ResolveResponse{Name: req.Name, Content: req.EditorContent}, nil
	case "#terminalLastCommand":
		return ResolveResponse{Name: req.Name, Content: req.LastTerminalCommand}, nil
	case "#codebase":
		if req.Arg == "" {
			return ResolveResponse{}, fmt.Errorf("%w: #codebase needs a query", ErrMissingArg)
		}
		if rag == nil {
			return ResolveResponse{}, ErrNoSearcher
		}
		snip, err := rag.Search(ctx, req.Arg, 6)
		if err != nil {
			return ResolveResponse{}, fmt.Errorf("chatvars: codebase search: %w", err)
		}
		return ResolveResponse{Name: req.Name, Content: snip}, nil
	case "#problems":
		return ResolveResponse{Name: req.Name, Content: strings.Join(req.Problems, "\n")}, nil
	case "#folder":
		return resolveFolder(req)
	case "#symbol":
		return ResolveResponse{}, fmt.Errorf("chatvars: #symbol requires extension-side resolution (v0.2.1)")
	default:
		return ResolveResponse{}, fmt.Errorf("%w: %s", ErrUnknownVariable, req.Name)
	}
}

func resolveFile(req ResolveRequest) (ResolveResponse, error) {
	if req.Arg == "" {
		return ResolveResponse{}, fmt.Errorf("%w: #file needs a path", ErrMissingArg)
	}
	abs, err := safeJoin(req.WorkspaceRoot, req.Arg)
	if err != nil {
		return ResolveResponse{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ResolveResponse{}, fmt.Errorf("chatvars: read file: %w", err)
	}
	truncated := false
	if len(b) > MaxFileBytes {
		b = b[:MaxFileBytes]
		truncated = true
	}
	content := string(b)
	if truncated {
		content += fmt.Sprintf("\n\n// [foundry-copilot: file truncated at %d bytes]", MaxFileBytes)
	}
	return ResolveResponse{Name: req.Name, Content: content}, nil
}

func resolveFolder(req ResolveRequest) (ResolveResponse, error) {
	if req.Arg == "" {
		return ResolveResponse{}, fmt.Errorf("%w: #folder needs a path", ErrMissingArg)
	}
	abs, err := safeJoin(req.WorkspaceRoot, req.Arg)
	if err != nil {
		return ResolveResponse{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return ResolveResponse{}, fmt.Errorf("chatvars: read folder: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var lines []string
	for i, e := range entries {
		if i >= MaxFolderEntries {
			lines = append(lines, fmt.Sprintf("... [truncated at %d entries]", MaxFolderEntries))
			break
		}
		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
		} else {
			lines = append(lines, e.Name())
		}
	}
	return ResolveResponse{Name: req.Name, Content: strings.Join(lines, "\n")}, nil
}

// safeJoin rejects absolute paths and any path with ".." segments before
// joining to root. The final path must remain within root.
//
// The absolute-path check is intentionally stricter than `filepath.IsAbs` so
// that the guard behaves identically on every host OS. `filepath.IsAbs` on
// Windows returns false for Unix-style rooted paths like "/etc/passwd" —
// without the explicit prefix check, such a path would silently be joined
// under the workspace root and could be resolved if the workspace happened
// to contain a matching tree. We therefore also reject:
//   - leading "/" or "\" (rooted on any platform)
//   - Windows drive-letter prefixes like "C:foo" or "C:/foo"
func safeJoin(root, rel string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("chatvars: workspace_root not set")
	}
	if filepath.IsAbs(rel) ||
		strings.HasPrefix(rel, "/") ||
		strings.HasPrefix(rel, `\`) ||
		(len(rel) >= 2 && rel[1] == ':') {
		return "", fmt.Errorf("%w: %q is absolute", ErrPathEscape, rel)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(clean, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q contains ..", ErrPathEscape, rel)
	}
	joined := filepath.Join(root, clean)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("chatvars: abs root: %w", err)
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("chatvars: abs join: %w", err)
	}
	if absJoined != absRoot && !strings.HasPrefix(absJoined, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: resolves outside workspace", ErrPathEscape)
	}
	return absJoined, nil
}
