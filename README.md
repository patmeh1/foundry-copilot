# foundry-copilot

A GitHub Copilot–style VS Code extension and standalone CLI/TUI coding harness whose model traffic is **hard-locked to Microsoft Foundry** (Azure AI Foundry) deployments.

> **Why it exists**: most Copilot-style assistants route to OpenAI, Anthropic, or Ollama. `foundry-copilot` enforces — by construction — that every chat, completion, embedding, and agent step goes to a Microsoft Foundry endpoint authenticated via Microsoft Entra ID. No API-key bypass. No "advanced provider" override. The lock lives in one Go module and every code path traverses it.

**📖 End users: jump straight to the [User Guide](docs/USER_GUIDE.md).**

## Architecture

```
VS Code Extension (TS thin shim)
  └── spawns ──▶ Go sidecar (cmd/sidecar)
                  ├── internal/foundry  (HARD LOCK + Entra auth)
                  ├── internal/agent    (ReAct loop, tools)
                  ├── internal/rag      (tree-sitter + chromem-go)
                  ├── internal/mcp      (client + server)
                  └── internal/rpc      (JSON-RPC 2.0 over stdio)
                                ▲
                                │ same core
                                ▼
                  cmd/harness  (Bubble Tea TUI binary)
```

- **Primary language: Go 1.22+** for the sidecar and standalone harness.
- TS is used **only** for the thin VS Code Extension Host shim (UI, lifecycle, settings) since the Extension API is Node.js-only.
- Communication: JSON-RPC 2.0 over stdio (`creachadair/jrpc2`).

## Hard Lock (the differentiator)

Three independent layers enforce "Foundry only":

1. **Endpoint allow-list** in `core/internal/foundry/lock.go` — compiled-in host-suffix list (`*.services.ai.azure.com`, `*.cognitiveservices.azure.com`, `*.openai.azure.com`, `*.inference.ml.azure.com`). An HTTP transport wrapper re-validates **every** outbound request.
2. **Auth issuer pinning** — `azidentity.DefaultAzureCredential` only. No API-key code path is compiled into the binary.
3. **Settings schema regex** in `extension/package.json` rejects non-Foundry URLs at the VS Code UI layer.

See `SECURITY.md` for the full security contract.

## Repository layout

```
.
├── core/                     # Go monorepo: sidecar + harness + tests
│   ├── cmd/sidecar/          # VS Code sidecar entry point
│   ├── cmd/harness/          # Standalone TUI binary
│   └── internal/
│       ├── foundry/          # CRITICAL — hard lock + Foundry client
│       ├── rpc/              # JSON-RPC server over stdio
│       ├── agent/            # Agent loop + completion shaping
│       ├── tools/            # Tool registry (read_file, edit, run, …)
│       ├── rag/              # Indexer, chunker, retriever
│       ├── mcp/              # MCP client + server
│       ├── config/           # Config (viper)
│       └── logx/             # Structured logging wrapper
├── extension/                # VS Code extension (TypeScript)
│   ├── src/
│   │   ├── extension.ts
│   │   ├── sidecar/          # Process manager + RPC client
│   │   ├── chat/             # Chat participant + webview
│   │   └── completions/      # Inline completion provider
│   ├── package.json          # Manifest + settings schema (with lock regex)
│   └── tsconfig.json
├── .github/workflows/        # CI + release
├── scripts/                  # Smoke tests, dev helpers
├── Makefile                  # Top-level dev targets
├── plan.md                   # Build plan (10 phases)
└── SECURITY.md               # Hard-lock contract
```

## Quick start (dev)

Prerequisites: Go ≥ 1.22, Node ≥ 20, an Azure subscription with a Foundry project and at least one chat model deployed.

```bash
# Install deps
make deps

# Build everything
make build

# Run hard-lock tests (must pass before anything else)
make test-lock

# Run all Go tests
make test

# Package a platform-specific VSIX for your machine
make vsix

# Build VSIXes for every platform at once (darwin/linux/win × amd64/arm64)
make vsix-all
```

## Install from a VSIX

Each release ships one VSIX per platform, each bundling only that platform's
sidecar + harness binaries (so artifacts stay small):

| Platform | File |
|---|---|
| macOS (Apple Silicon) | `foundry-copilot-darwin-arm64.vsix` |
| macOS (Intel) | `foundry-copilot-darwin-x64.vsix` |
| Linux (x64) | `foundry-copilot-linux-x64.vsix` |
| Linux (arm64) | `foundry-copilot-linux-arm64.vsix` |
| Windows (x64) | `foundry-copilot-win32-x64.vsix` |

```bash
code --install-extension foundry-copilot-darwin-arm64.vsix
```

Then set `foundryCopilot.endpoint` in VS Code settings and run `az login`.

## Releases

Tagging `v*.*.*` on `main` runs `.github/workflows/release.yml`, which builds
all five VSIXes and attaches them to a GitHub Release.

## Authentication

The extension uses `azidentity.DefaultAzureCredential`, which tries in order:
1. Environment variables (`AZURE_*`)
2. Workload identity (in cluster)
3. Managed identity (in Azure)
4. Azure CLI (`az login`)
5. Azure Developer CLI (`azd auth login`)

For local dev, run `az login` once.

## Configuration

In VS Code settings:

| Setting | Description |
|---------|-------------|
| `foundryCopilot.endpoint` | Foundry endpoint URL. **Must match** the allow-list regex. |
| `foundryCopilot.chatModelDeployment` | Deployment name for chat. |
| `foundryCopilot.completionModelDeployment` | Deployment name for inline FIM completions. |
| `foundryCopilot.embeddingDeployment` | Embedding deployment for RAG. |

## License

MIT — see [LICENSE](LICENSE).
