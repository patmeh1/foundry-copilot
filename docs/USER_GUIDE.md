# Foundry Copilot — User Guide

A GitHub Copilot–style assistant for VS Code that **only** talks to your
Microsoft Foundry (Azure AI Foundry) deployments. No external model
providers, no API keys, no escape hatches.

This guide walks you from zero to a working install. For the security
contract behind the hard lock, see [SECURITY.md](../SECURITY.md). For
the build plan, see [plan.md](../plan.md).

---

## 1. Prerequisites

| Requirement | Why |
|---|---|
| VS Code **1.95+** | `chatParticipant` API |
| An Azure subscription with a **Microsoft Foundry / Azure AI** resource | The lock allows only `*.services.ai.azure.com`, `*.cognitiveservices.azure.com`, `*.openai.azure.com`, `*.inference.ml.azure.com` |
| At least one **chat model deployment** (e.g. `gpt-4o`, `gpt-4o-mini`) | Required for `@foundry` chat and `/agent` |
| Azure CLI (`az login`) — or any other `DefaultAzureCredential` source | Auth. No API-key bypass exists. |
| *Optional:* a **completion** deployment | Enables inline (ghost-text) completions |
| *Optional:* an **embedding** deployment | Enables `Refresh Workspace Index` and the `rag_search` agent tool |

> The four allowed host suffixes are compiled into the binary. If you
> point the extension at anything else (including a corporate proxy
> URL), the sidecar refuses to start.

---

## 2. Install

1. Download the VSIX for your platform from the latest [GitHub Release](https://github.com/patmeh1/foundry-copilot/releases):

   | Platform | File |
   |---|---|
   | macOS (Apple Silicon) | `foundry-copilot-darwin-arm64.vsix` |
   | macOS (Intel) | `foundry-copilot-darwin-x64.vsix` |
   | Linux (x64) | `foundry-copilot-linux-x64.vsix` |
   | Linux (arm64) | `foundry-copilot-linux-arm64.vsix` |
   | Windows (x64) | `foundry-copilot-win32-x64.vsix` |

2. Install via the CLI:

   ```bash
   code --install-extension foundry-copilot-darwin-arm64.vsix
   ```

   Or in VS Code: **Extensions** view → **`…` menu** → **Install from VSIX…**

3. Reload the window.

---

## 3. Sign in to Azure

The extension uses `DefaultAzureCredential`, which tries (in order):
environment variables → workload identity → managed identity →
Azure CLI → Azure Developer CLI.

The easiest path is the CLI:

```bash
az login
# Optional: pick a tenant
az login --tenant <your-tenant-id>
```

If your Foundry resource lives in a tenant different from your default,
run `az account set --subscription <id>` so the bearer token is
issued for the right subscription.

---

## 4. Configure the extension

Open **Settings** (`⌘,` / `Ctrl+,`) and search for **Foundry Copilot**.
Set at least these three:

| Setting | Example value |
|---|---|
| `foundryCopilot.endpoint` | `https://my-resource.services.ai.azure.com` |
| `foundryCopilot.chatDeployment` | `gpt-4o` |
| `foundryCopilot.logLevel` | `info` |

Optional:

| Setting | What it does |
|---|---|
| `foundryCopilot.completionDeployment` | Enables inline ghost-text completions |
| `foundryCopilot.embeddingDeployment` | Enables RAG indexing + `rag_search` |
| `foundryCopilot.agent.maxSteps` | Cap tool-use steps (default 12, max 64) |
| `foundryCopilot.agent.allowWrite` | Let `/agent` write files (default **off**) |
| `foundryCopilot.agent.allowShell` | Let `/agent` run shell commands (default **off**) |
| `foundryCopilot.mcp.servers` | Array of MCP stdio servers — see §8 |

> The `endpoint` setting has a regex check that **rejects** any URL
> outside the Foundry allow-list. If VS Code shows
> *"Endpoint must be HTTPS and end in a Microsoft Foundry domain"*,
> double-check the host suffix.

---

## 5. Verify the install

Run **Foundry Copilot: Ping Sidecar** from the Command Palette
(`⌘⇧P` / `Ctrl+Shift+P`).

You should see a notification like:
> *Sidecar OK — v0.1.0*

If it fails, run **Foundry Copilot: Show Output Channel** and check
for one of:

- *"endpoint not configured"* → set `foundryCopilot.endpoint`
- *"DefaultAzureCredential: failed to acquire token"* → `az login`
- *"host not in foundry allow-list"* → your endpoint host suffix is wrong

---

## 6. Use the chat participant

Open the Copilot Chat panel (or VS Code's built-in Chat view in 1.95+)
and address the assistant with `@foundry`:

```
@foundry How does the agent loop call MCP tools?
```

Responses stream token-by-token from your Foundry deployment.

### Slash commands

| Command | What it does | Selection-aware |
|---|---|---|
| `@foundry /explain` | Explain the selected code or active file | yes |
| `@foundry /fix` | Diagnose and fix bugs in the selection | yes |
| `@foundry /tests` | Generate unit tests for the selection | yes |
| `@foundry /doc` | Add documentation comments | yes |
| `@foundry /agent <task>` | Run the tool-using agent loop on your workspace | no |

Each slash command builds a tightly-scoped prompt from the active
editor's selection (or the whole file, clipped to 16 KiB if it's huge).

---

## 7. Inline completions

If you set `foundryCopilot.completionDeployment`, the extension installs
a provider that suggests ghost-text completions as you type. Accept
with **Tab**, dismiss with **Esc**.

> Tip: not every Foundry deployment is good at FIM (fill-in-the-middle).
> Models like `gpt-4o-mini` work; for best latency use a small
> deployment dedicated to completions.

---

## 8. Workspace RAG (optional)

If you set `foundryCopilot.embeddingDeployment`, you can index your
workspace so the agent can search it:

1. Open the workspace you want to index.
2. Run **Foundry Copilot: Refresh Workspace Index**.
3. A progress notification shows how many files / chunks were embedded
   (this can take a minute on a big repo).
4. The index lives in `~/.foundry-copilot/index/store.gob` per workspace.

Once built, `@foundry /agent` automatically gets a `rag_search` tool
and can pull snippets from your codebase.

---

## 9. Agent mode (tool use)

```
@foundry /agent Find every place we call openai-go directly and consolidate them into one helper.
```

The agent loop runs **up to `foundryCopilot.agent.maxSteps`** rounds of:

1. Model decides which tool to call.
2. Tool runs (output is streamed back).
3. Model sees the result and either calls another tool or returns the final answer.

### Built-in tools

| Tool | Always on? | What it does |
|---|---|---|
| `fs_read` | yes | Read a file (path scoped to workspace) |
| `code_search` | yes | Recursive substring search with skip-dir denylist |
| `rag_search` | only if index built | Vector search over the indexed workspace |
| `fs_write` | only if `agent.allowWrite=true` | Create/overwrite files |
| `shell` | only if `agent.allowShell=true` | Run a shell command (30 s timeout, 16 KiB output cap) |
| `mcp__{server}__{tool}` | per MCP entry | Any tool exposed by a connected MCP server |

> Both `fs_write` and `shell` are **off by default**. Turn them on
> deliberately when you want the agent to act on your machine.

---

## 10. MCP servers (optional)

You can wire any number of stdio Model Context Protocol servers and
their tools become available to `/agent` automatically.

Example `settings.json`:

```jsonc
{
  "foundryCopilot.mcp.servers": [
    {
      "id": "fs",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "${workspaceFolder}"]
    },
    {
      "id": "github",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "${env:GH_TOKEN}" }
    }
  ]
}
```

The extension launches each entry on activation. Tool names are
namespaced — e.g. `mcp__github__create_issue`.

Use **Foundry Copilot: Reconnect MCP Servers** after editing the list
or if a server died.

---

## 11. Standalone harness (no VS Code)

Every VSIX bundles a sister binary `foundry-copilot` (or `.exe`) at
`extension/bin/<platform>/`. Drop it in your `$PATH` and you get the
same chat / agent / index pipeline in a Bubble Tea TUI:

```bash
foundry-copilot version
foundry-copilot init                # check config
foundry-copilot chat                # TUI chat
foundry-copilot agent               # TUI with tools
foundry-copilot index .             # build RAG index for the cwd
```

It reads the same env vars / config the extension's sidecar uses
(`FOUNDRY_COPILOT_ENDPOINT`, `FOUNDRY_COPILOT_CHAT_DEPLOYMENT`, …).
The hard lock is identical.

### TUI keybindings

| Key | Action |
|---|---|
| **Enter** | Send |
| **Shift+Enter** | Newline in the input |
| **Ctrl+T** | Toggle chat ⇄ agent mode |
| **Ctrl+C** | Quit |

---

## 12. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| *"Endpoint must be HTTPS and end in a Microsoft Foundry domain"* | Your URL doesn't match the allow-list — check the host suffix |
| *"endpoint not configured"* in the output channel | `foundryCopilot.endpoint` is empty |
| *"DefaultAzureCredential: failed to acquire token"* | Run `az login`, or set `AZURE_TENANT_ID` / `AZURE_CLIENT_ID` / `AZURE_CLIENT_SECRET` env vars |
| *"deployment not found"* | The chat / completion / embedding deployment name doesn't exist in your Foundry resource |
| *"host not in foundry allow-list"* | The sidecar rejected an outbound URL — this is the lock doing its job; do **not** try to disable it |
| `@foundry` doesn't appear in Chat | VS Code < 1.95, or extension failed to activate — open Output → **Foundry Copilot** |
| Sidecar keeps restarting | Run **Foundry Copilot: Show Output Channel** for the panic; binary mismatch (wrong platform VSIX) is the most common cause |
| Inline completions never show | `foundryCopilot.completionDeployment` is unset, or the model doesn't support fast completions |
| `/agent` says *"no rag index"* | Run **Foundry Copilot: Refresh Workspace Index** first |

For deeper debugging set `foundryCopilot.logLevel` to `debug` and
restart the window.

---

## 13. Uninstall

```bash
code --uninstall-extension patmeh1.foundry-copilot
rm -rf ~/.foundry-copilot   # optional: removes cached index + config
```

---

## 14. License & security

MIT — see [LICENSE](../LICENSE). The hard-lock guarantees and threat
model live in [SECURITY.md](../SECURITY.md).
