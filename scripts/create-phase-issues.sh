#!/usr/bin/env bash
# Create phase labels and one tracking issue per phase.
# Idempotent: skips labels/issues that already exist.
set -euo pipefail
export GH_PAGER=cat

REPO="patmeh1/foundry-copilot"

# --- labels -----------------------------------------------------------------
mklabel() {
  local name="$1" color="$2" desc="$3"
  if gh -R "$REPO" label list --json name -q '.[].name' | grep -qx "$name"; then
    echo "label $name exists, skipping"
  else
    gh -R "$REPO" label create "$name" --color "$color" --description "$desc"
  fi
}

mklabel "phase:0-scaffolding"      "0e8a16" "Phase 0 — repo scaffolding"
mklabel "phase:1-go-core"          "1d76db" "Phase 1 — Go core foundations"
mklabel "phase:2-extension-shell"  "1d76db" "Phase 2 — VS Code extension shell"
mklabel "phase:3-chat-mvp"         "5319e7" "Phase 3 — Chat MVP"
mklabel "phase:4-completions"      "5319e7" "Phase 4 — Inline completions"
mklabel "phase:5-agent"            "d93f0b" "Phase 5 — Agent mode + tools"
mklabel "phase:6-slash"            "fbca04" "Phase 6 — Slash commands"
mklabel "phase:7-rag"              "d93f0b" "Phase 7 — RAG indexing"
mklabel "phase:8-mcp"              "d93f0b" "Phase 8 — MCP client + server"
mklabel "phase:9-harness"          "5319e7" "Phase 9 — Standalone TUI harness"
mklabel "phase:10-release"         "b60205" "Phase 10 — Packaging + release"
mklabel "hard-lock"                "b60205" "Touches the Foundry hard-lock"

# --- issues -----------------------------------------------------------------
mkissue() {
  local title="$1" labels="$2" body="$3"
  if gh -R "$REPO" issue list --search "in:title \"$title\"" --json title -q '.[].title' | grep -qx "$title"; then
    echo "issue '$title' exists, skipping"
    return
  fi
  gh -R "$REPO" issue create --title "$title" --label "$labels" --body "$body"
}

mkissue "Phase 0 — Repo scaffolding" "phase:0-scaffolding" \
"Monorepo bootstrap. See plan.md.

**Done when:**
- [x] Root README, LICENSE (MIT), SECURITY.md, plan.md
- [x] .gitignore + Makefile with phased targets
- [ ] core/ Go module initialised (go.mod)
- [ ] extension/ TS workspace initialised (package.json, tsconfig.json)
- [ ] .github/workflows/ci.yml builds + tests both halves on push"

mkissue "Phase 1 — Go core foundations" "phase:1-go-core,hard-lock" \
"Foundry client, **hard lock**, auth, JSON-RPC server, config, logging. See plan.md.

**Done when:**
- [ ] \`core/internal/foundry/lock.go\` — host-suffix allow-list + locked transport
- [ ] \`core/internal/foundry/lock_test.go\` — table-driven, covers rejection of openai/anthropic/localhost/IPs/suffix-substring attack
- [ ] \`core/internal/foundry/auth.go\` — DefaultAzureCredential + issuer pre-flight
- [ ] \`core/internal/foundry/client.go\` — azopenai wrapper (chat, embed, stream)
- [ ] \`core/internal/rpc/server.go\` — jrpc2 over stdio with ping/version/chat/stream
- [ ] \`core/internal/config/config.go\` — viper config
- [ ] \`core/internal/logx/logx.go\` — slog wrapper (stderr only; stdout reserved for RPC)
- [ ] All tests pass: \`make test-go\`"

mkissue "Phase 2 — VS Code extension shell" "phase:2-extension-shell,hard-lock" \
"Activation, sidecar process manager, settings schema with lock regex. See plan.md.

**Done when:**
- [ ] \`extension/package.json\` — manifest, contributes.configuration with endpoint regex, chat participant declared
- [ ] \`extension/src/extension.ts\` — activate()/deactivate(), wires sidecar
- [ ] \`extension/src/sidecar/process.ts\` — spawn, restart-on-crash, version probe
- [ ] \`extension/src/sidecar/rpc.ts\` — typed JSON-RPC client
- [ ] \`extension/tsconfig.json\` + esbuild bundling
- [ ] \`make build-ext\` succeeds"

mkissue "Phase 3 — Chat MVP (participant + streaming + webview)" "phase:3-chat-mvp" \
"End-to-end chat through Foundry with streaming tokens.

**Done when:**
- [ ] Chat participant @foundry registered
- [ ] sidecar method \`chat/stream\` wired to azopenai streaming
- [ ] React webview chat panel (Vite-built) themed with VS Code tokens
- [ ] On-disk session persistence under \`~/.foundry-copilot/sessions/\`"

mkissue "Phase 4 — Inline completions" "phase:4-completions" \
"Ghost-text completions via Foundry FIM-capable deployment.

**Done when:**
- [ ] \`InlineCompletionItemProvider\` registered
- [ ] sidecar \`complete/inline\` with debounce + cancellation
- [ ] p50 latency ≤ 600 ms locally"

mkissue "Phase 5 — Agent mode + tool registry" "phase:5-agent" \
"ReAct loop with parallel tool calls and approval UI.

**Done when:**
- [ ] Tools: read_file, write_file, apply_patch, run_terminal, grep, list_dir, codebase_search
- [ ] JSON Schema declared via invopop/jsonschema struct tags
- [ ] sidecar \`agent/run\` with step limits + cancellation
- [ ] Extension-side approval modal for destructive tools"

mkissue "Phase 6 — Slash commands" "phase:6-slash" \
"\`/explain\` \`/fix\` \`/tests\` \`/docs\` on the chat participant.

**Done when:**
- [ ] All four commands registered with descriptions
- [ ] Each captures relevant context (selection / diagnostics / active file) and routes to chat/stream"

mkissue "Phase 7 — RAG indexing (tree-sitter + chromem-go)" "phase:7-rag" \
"Code-aware embeddings + retrieval.

**Done when:**
- [ ] Extension file watcher forwards events to sidecar \`index/refresh\`
- [ ] tree-sitter chunker (Go/TS/Python/Rust/Java)
- [ ] chromem-go vector store persisted under \`.foundry-copilot/index/\`
- [ ] Hybrid retriever (vector + BM25), \`.foundryignore\` honoured"

mkissue "Phase 8 — MCP client + server" "phase:8-mcp,hard-lock" \
"Sidecar is **both** MCP client (external servers) and MCP server (exposes Foundry-backed primitives). Lock still applies.

**Done when:**
- [ ] mark3labs/mcp-go client connects to configured external servers (stdio/SSE/HTTP)
- [ ] External tools merged into agent tool registry
- [ ] mark3labs/mcp-go server exposes codebase_search / RAG retrieve
- [ ] All MCP outbound traffic still goes through locked transport"

mkissue "Phase 9 — Standalone TUI harness" "phase:9-harness" \
"cobra + bubbletea binary reusing the same core/.

**Done when:**
- [ ] \`cmd/harness\` builds a single static binary
- [ ] Subcommands: \`init\`, \`chat\`, \`index\`, \`version\`
- [ ] TUI chat loop completes a real turn against Foundry"

mkissue "Phase 10 — Packaging, signing, release" "phase:10-release" \
"Multi-platform VSIX + release pipeline.

**Done when:**
- [ ] goreleaser builds sidecar+harness for 5 targets
- [ ] vsce platform-specific VSIXes published per target
- [ ] GitHub Actions release.yml triggered by tag
- [ ] (optional) Apple notarization + Authenticode signing wired"

echo
echo "Created labels + issues. Listing:"
gh -R "$REPO" issue list --limit 20
