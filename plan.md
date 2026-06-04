# Plan: Foundry-Locked Copilot Clone (VS Code + Go)

A VS Code extension that clones the GitHub Copilot experience (chat, inline completions, agent mode, slash commands, RAG, MCP) but where every model call is hard-locked to deployments in **Microsoft Foundry** (Azure AI Foundry). Architecture is a thin TypeScript extension shim that spawns a Go sidecar; the Go sidecar owns all model traffic, the agent loop, RAG, and MCP. The same Go core ships as a bonus standalone TUI coding-harness binary.

## Why this shape

- VS Code Extension Host is Node.js-only — TS is unavoidable for the IDE surface. **Go is the primary language** (sidecar + harness); TS is intentionally minimized to UI bindings and process lifecycle.
- Hard lock is enforced in **one Go module** (`internal/foundry`) that every code path must go through. No model traffic ever originates in the TS layer.
- Single Go core → two frontends (VS Code sidecar + standalone CLI/TUI) maximizes reuse and gives a credible Aider/Crush-style fallback for non-VS-Code users.

## Recommended Tech Stack

**Required**
- **Go 1.22+** — sidecar + CLI/TUI harness core
- **TypeScript 5.x** — VS Code extension shim
- **Node 20 LTS / pnpm** — TS toolchain

**Go libraries** (all mature, actively maintained as of 2026)
- `github.com/Azure/azure-sdk-for-go/sdk/azidentity` — `DefaultAzureCredential`, device-code, managed identity
- `github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai` — chat completions, embeddings, streaming; covers Foundry OpenAI + Phi/Mistral/Llama/DeepSeek deployments
- Direct `net/http` calls — Foundry Agents Service (Go SDK not yet GA; REST is stable)
- `github.com/creachadair/jrpc2` — JSON-RPC 2.0 over stdio (extension ↔ sidecar)
- `github.com/mark3labs/mcp-go` — MCP server **and** client
- `github.com/spf13/cobra` + `github.com/spf13/viper` — CLI/config (harness mode)
- `github.com/charmbracelet/bubbletea` + `lipgloss` — TUI for harness mode
- `github.com/philippgille/chromem-go` — pure-Go embedded vector store (no native deps, ships in a single binary)
- `github.com/smacker/go-tree-sitter` — code-aware chunking for RAG (Go/TS/Python/Rust/Java grammars)
- `log/slog` (stdlib) — structured logging
- `github.com/stretchr/testify` — assertions
- `github.com/goreleaser/goreleaser` — multi-platform release pipeline

**TypeScript libraries**
- `@types/vscode` ≥ 1.95 — Chat Participant, Language Model Provider, native MCP APIs
- `esbuild` — extension bundling
- `vite` + `react` — chat panel webview
- `@vscode/vsce` — platform-specific VSIX packaging

## Hard-Lock Design (core differentiator)

Three independent enforcement layers:

1. **Endpoint allow-list** (`internal/foundry/lock.go`) — compiled-in host-suffix list (`*.services.ai.azure.com`, `*.cognitiveservices.azure.com`, `*.openai.azure.com`, `*.inference.ml.azure.com`); HTTP transport wrapper re-validates every request, fails closed.
2. **Auth issuer pinning** — `azidentity.DefaultAzureCredential` only; no API-key code path exists. JWT `iss` checked pre-flight.
3. **Settings schema regex** in `extension/package.json` blocks invalid URLs at the UI layer. No bypass setting. No env-var override.

## Phased Implementation

### Phase 0 — Scaffolding
Monorepo layout (`core/` Go, `extension/` TS), CI skeleton.

### Phase 1 — Go core foundations
Foundry client, hard-lock + auth (table-driven tests), JSON-RPC server skeleton (`creachadair/jrpc2`), config (`viper`) + structured logging (`log/slog`).

### Phase 2 — VS Code extension shell
Activation, sidecar process manager, settings schema, chat participant registration stub.

### Phase 3 — Chat MVP
Chat participant → sidecar `chat/stream` → Foundry with token streaming. Custom React webview panel. On-disk session persistence.

### Phase 4 — Inline completions
`InlineCompletionItemProvider`, FIM request shaping, debounce + cancellation.

### Phase 5 — Agent mode + tools
Tool registry (`read_file`, `write_file`, `apply_patch`, `run_terminal`, `grep`, `list_dir`, `codebase_search`), ReAct loop with parallel tool calls, extension-side approval UI.

### Phase 6 — Slash commands
`/explain`, `/fix`, `/tests`, `/docs` on the chat participant.

### Phase 7 — RAG indexing
File watcher → tree-sitter chunker → Foundry embeddings → `chromem-go` store; hybrid retrieval; `.foundryignore`.

### Phase 8 — MCP integration
Sidecar as MCP client (consumes external servers) **and** MCP server (lets other clients use Foundry-backed primitives).

### Phase 9 — Standalone TUI harness
`cmd/harness` reusing core; `bubbletea` chat TUI.

### Phase 10 — Packaging, signing, release
`goreleaser` builds 5 targets → platform-specific VSIXes via `vsce` → Apple notarization + Authenticode → tag-triggered GitHub Actions release.

## Verification

1. Hard-lock unit tests (table-driven) — including rejection cases for `api.openai.com`, `api.anthropic.com`, `localhost:11434`, `127.0.0.1`, suffix-substring attack `example.com.services.ai.azure.com.attacker.com`.
2. Hard-lock integration test — sidecar with malicious endpoint env override returns `ErrEndpointNotFoundry`.
3. Foundry live smoke (gated by `FOUNDRY_SMOKE=1`).
4. Extension activation test via `@vscode/test-electron`.
5. End-to-end chat trace: `@foundry hello` → streams → persists → restores.
6. Inline completion p50 latency ≤ 600 ms; cancellation works.
7. Agent edit confirmation: destructive tool triggers approval modal.
8. RAG correctness: index `core/`, ask "where is the hard lock enforced?" → cites `lock.go`.
9. MCP roundtrip via external `mcp-server-time`.
10. Cross-platform packaging produces 5 VSIXes.
11. Harness binary completes a turn against Foundry.

## Decisions

- Single Go monorepo (`core/`) drives both sidecar and TUI — hard lock lives in exactly one place.
- JSON-RPC over stdio (not gRPC/HTTP).
- `mark3labs/mcp-go` over rolling our own.
- `chromem-go` over sqlite-vec — pure Go, no CGo, single static binary.
- Platform-specific VSIX (5 targets) bundling the Go binary — no first-run download.
- Custom webview chat panel alongside native Chat Participant — needed for a true Copilot clone UX.

**Out of scope**: fine-tuning, model deployment workflows, telemetry dashboards, multi-user/team features, non-Foundry providers (by design).
