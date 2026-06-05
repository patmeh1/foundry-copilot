# Foundry Copilot — User Guide

**Version 0.2.1.** A GitHub Copilot–style assistant for VS Code that **only**
talks to your Microsoft Foundry (Azure AI Foundry) deployments. No external
model providers, no API keys, no escape hatches.

This guide walks you from zero to a working install. For the security
contract behind the hard lock, see [SECURITY.md](../SECURITY.md). For
the build plan, see [plan.md](../plan.md). Prefer screenshots? See the
[Visual Tour](VISUAL_TOUR.md) for a picture-first walk through every
surface.

---

## What's new in 0.2.1

Foundry Copilot 0.2.x is a major surface-area expansion over 0.1.x. The
short version: it now covers every AI surface VS Code has — chat, inline
completions, inline edits, terminal, notebooks, SCM, fine-tuning,
telemetry, billing — and every one of them streams through the same Go
sidecar locked to your Foundry endpoint.

- **Own chat view, own activity-bar container.** Foundry Copilot lives at
  `⌘⌥I` / `Ctrl+Alt+I` (shield icon in the activity bar). It does **not**
  participate in VS Code's built-in chat panel — see [§6](#6-use-the-foundry-chat-view).
- **BAA boundary guard.** On activation Foundry Copilot detects and
  disables `GitHub.copilot`, `GitHub.copilot-chat`, `GitHub.copilot-workspace`,
  and `GitHub.copilot-labs` (configurable). Status bar shows enforcement state.
  See [§6a](#6a-baa-boundary-guard).
- **Inline / quick / terminal / notebook chat** (§7–10).
- **SCM commit messages + PR descriptions** (§11).
- **“Diagnose this test failure”** code action (§12).
- **Next Edit Suggestions (NES)** — multi-line inline edits, opt-in (§13).
- **Activity-bar tree views** for Deployments, Telemetry, Fine-tuning,
  Billing (§14).
- **Walkthrough**: a one-time “Foundry BAA Boundary” walkthrough explains
  why GitHub Copilot is disabled.

If you used 0.1.x, the things that have **changed** are:

- `@foundry` no longer exists in VS Code's chat panel — use the Foundry
  chat view instead.
- Slash commands (`/explain`, `/fix`, …) have been removed. Their
  workflows are now first-class commands or code actions (see §7–12).
- Settings keys were renamed: `chatModelDeployment` → `chatDeployment`,
  `completionModelDeployment` → `completionDeployment`.

---

## 1. Prerequisites

| Requirement | Why |
|---|---|
| VS Code **1.95+** | Activity-bar webview container + inline-edit / NES APIs |
| An Azure subscription with a **Microsoft Foundry / Azure AI** resource | The lock allows only `*.services.ai.azure.com`, `*.cognitiveservices.azure.com`, `*.openai.azure.com`, `*.inference.ml.azure.com` |
| At least one **chat model deployment** (e.g. `gpt-4o`, `gpt-4o-mini`, `gpt-5.4`) | Required for the Foundry chat view and every chat-style surface |
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
> *Sidecar OK — v0.2.1*

Then check the status bar in the bottom-right corner. Two Foundry Copilot
items should appear:

- **🛡️ BAA: enforced** — green / theme-default background means the BAA
  boundary is healthy (no conflicting GitHub Copilot extensions enabled).
  A red **⚠️ BAA: conflict** means a conflicting extension is still active;
  click for options.
- **Foundry Copilot** chat indicator — click to focus the chat view.

If Ping fails, run **Foundry Copilot: Show Output Channel** and check
for one of:

- *"endpoint not configured"* → set `foundryCopilot.endpoint`
- *"DefaultAzureCredential: failed to acquire token"* → `az login`
- *"host not in foundry allow-list"* → your endpoint host suffix is wrong

---

## 6. Use the Foundry chat view

Forge Copilot ships its own chat view in a dedicated activity-bar
container (shield icon 🛡️ labelled **Foundry Copilot**). This is the
primary way to chat with your Foundry model.

- **Open it** — press `⌘⌥I` (macOS) / `Ctrl+Alt+I` (Windows / Linux),
  click the shield icon in the activity bar, or run **Foundry Copilot:
  Focus Chat** from the command palette.
- **Type a prompt and press Enter.** Tokens stream back live.
- **Multiple threads.** The dropdown at the top of the view lets you
  switch between conversation threads. Each thread is persisted on disk.
- **Deployments side-panel.** Below the chat view is a **Deployments**
  tree that lists every model deployment on your Foundry endpoint with
  capability, TPM, and version info — useful for picking the right
  `chatDeployment`.

### Why not the built-in VS Code chat panel?

Forge Copilot intentionally does **not** register a chat participant
(no `@foundry` in the built-in chat). The built-in chat panel ships with
GitHub Copilot Chat, which has no Business Associate Agreement.
Participating there would let prompts route through whatever LM provider
the user (or another extension) picks — defeating the entire point of
the Foundry hard-lock. See [SECURITY.md](../SECURITY.md) and core tenant
#3 in the [README](../README.md#core-tenants).

If the title-bar chat input is showing, Foundry Copilot will hide it on
activation by setting `chat.commandCenter.enabled = false` at workspace
scope. Opt out via `foundryCopilot.byoChat.hideBuiltInChat = false`.

---

## 6a. BAA boundary guard

On activation, Foundry Copilot scans for installed GitHub Copilot
extensions and (by default) disables them at the workspace scope:

| Extension ID | Why disabled |
|---|---|
| `GitHub.copilot` | Inline completions go to GitHub, no BAA |
| `GitHub.copilot-chat` | Provides the built-in chat panel that would intercept prompts |
| `GitHub.copilot-workspace` | Agent loop without BAA |
| `GitHub.copilot-labs` | Experimental features without BAA |

The status bar item updates accordingly:

- **🛡️ BAA: enforced** — no conflicts, you're good.
- **⚠️ BAA: conflict** — a conflicting extension is still enabled.
  Click for **Disable now / Show details / Configure policy**.
- **🛡️ BAA: off** — enforcement is set to `off`; the boundary is **not**
  protecting you. Red background.

Relevant settings:

| Setting | Default | What it does |
|---|---|---|
| `foundryCopilot.baa.enforcement` | `auto` | `auto` (silently disable), `prompt` (ask once), `off` (do nothing) |
| `foundryCopilot.baa.target` | `workspace` | `workspace` or `global` scope for the disable |
| `foundryCopilot.baa.conflictingExtensions` | `[]` | Additional extension IDs to treat as conflicts |

Run **Foundry Copilot: BAA — Disable Conflicting Extensions Now** or
**Foundry Copilot: BAA — Recheck Status** from the command palette to
re-run the guard.

For the rationale and threat model see the one-time walkthrough
(**Help → Get Started → Foundry BAA Boundary**) or
[SECURITY.md](../SECURITY.md).

---

## 7. Inline completions

If you set `foundryCopilot.completionDeployment`, the extension installs
a provider that suggests ghost-text completions as you type. Accept
with **Tab**, dismiss with **Esc**.

> Tip: not every Foundry deployment is good at FIM (fill-in-the-middle).
> Models like `gpt-4o-mini` work; for best latency use a small
> deployment dedicated to completions.

### Next Edit Suggestions (NES)

NES is an **opt-in** multi-line inline edit predictor. After you make
an edit, NES proposes the next coherent edit a few lines away (e.g.
rename, then update the same rename at the call site). Enable with:

```jsonc
{
  "foundryCopilot.nes.enabled": true
}
```

NES adds latency; off by default. Accept like a regular inline
completion (Tab).

---

## 8. Inline chat (`⌘I` / `Ctrl+I`)

With a selection (or just the cursor) in an editor, press `⌘I` to open
an inline chat input below the line. Ask things like:

- *"Refactor this to use options pattern"*
- *"Add a docstring"*
- *"Inline this variable"*

The diff is staged in a preview, and you can apply or reject. The
selected code is sent as the focus block; the surrounding file is
included as context up to a budget.

---

## 9. Quick chat (`⌘⇧⌥I` / `Ctrl+Shift+Alt+I`)

A one-shot conversational prompt that streams into a dedicated output
channel without changing your editor focus. Useful for quick
lookups ("what does `LISTEN/NOTIFY` do in Postgres?") without
opening the full chat view.

Leaves no thread state. Repeat invocations open separate output
channels so you can compare answers.

---

## 10. Terminal chat

From the integrated terminal, run **Foundry Copilot: Terminal Chat**
from the command palette. Foundry Copilot reads the last terminal
command (and its output, if visible) and asks for a description of
what you want.

The response is a single-line command that the extension stages with
a confirmation prompt before running. Type `y` to execute, anything
else to cancel.

---

## 11. Notebook chat

On a Jupyter / `.ipynb` cell, the command palette offers:

- **Foundry Copilot: Ask About This Cell** — explanatory chat scoped to the cell.
- **Foundry Copilot: Fix This Cell** — proposes a corrected cell with a diff.
- **Foundry Copilot: Generate Tests For This Cell** — emits a tests cell.

All three send the cell content plus the notebook's outputs to your
chat deployment.

---

## 12. SCM — commit messages + PR descriptions

### Generate Commit Message

In the Source Control view, with the Git provider active, click the
**✨ Foundry Copilot: Generate Commit Message** icon in the title bar
(or run the command of the same name).

Forge Copilot reads the staged diff (`git diff --cached`), sends it
to your chat deployment with a Conventional Commits prompt, and streams
the message into the commit input box. The diff is clipped at 16 KiB.

### Generate PR Description

Run **Foundry Copilot: Generate PR Description**. Foundry Copilot
finds your branch's merge-base against the upstream branch, sends the
range diff and commit list to the chat deployment, and opens the
result in a new editor for you to copy into your PR.

---

## 13. Diagnose test failure

When a test fails, click the lightbulb in the gutter and pick
**Foundry Copilot: Diagnose Test Failure**. Foundry Copilot reads the
failing test, its file, and the failure output, then streams a
diagnosis + suggested fix into an output channel.

Works with VS Code's native test results (any language's adapter).

---

## 14. Activity-bar tree views

Forge Copilot ships four tree views accessible from the activity-bar
container (shield icon) and the command palette:

| View | Command | Shows |
|---|---|---|
| **Chat** | `Foundry Copilot: Focus Chat` | The chat surface (§6) |
| **Deployments** | `Foundry Copilot: Open Deployments View` | Every model deployment on your endpoint: name, capability, TPM, version, SKU |
| **Telemetry** | `Foundry Copilot: Open Telemetry View` | Recent Foundry calls (latency, tokens, deployment, error). Click a row for full JSON. |
| **Fine-tuning** | `Foundry Copilot: Open Fine-tuning View` | Your fine-tune jobs: status, base model, training file, deployment status |
| **Billing** | `Foundry Copilot: Open Billing View` | Estimated daily and monthly spend from the local telemetry store |

### Local telemetry store

Every Foundry call is appended (by default) to a local JSONL file under
your user config dir. Disable with:

```jsonc
{ "foundryCopilot.telemetry.localStore": false }
```

The Telemetry and Billing views read from this store — nothing is sent
off-machine.

### Optional OTLP export

To also export to an OTLP HTTP collector, set **both**:

```jsonc
{
  "foundryCopilot.telemetry.otlpEndpoint": "https://otlp.mycorp.local/v1/traces",
  "foundryCopilot.telemetry.otlpAllowedHosts": ["otlp.mycorp.local"]
}
```

The exporter refuses to send unless the endpoint host (dot-anchored)
appears in `otlpAllowedHosts`. Same defence-in-depth pattern as the
Foundry lock.

### Daily budget alert

Set `foundryCopilot.billing.dailyBudgetUsd` to a positive number to
fire a one-time notification when your tracked Foundry spend for the
current UTC day crosses the budget. `0` (default) disables.

---

## 15. Workspace RAG (optional)

If you set `foundryCopilot.embeddingDeployment`, you can index your
workspace so the agent can search it:

1. Open the workspace you want to index.
2. Run **Foundry Copilot: Refresh Workspace Index**.
3. A progress notification shows how many files / chunks were embedded
   (this can take a minute on a big repo).
4. The index lives in `~/.foundry-copilot/index/store.gob` per workspace.

Once built, the `rag_search` tool is available to the agent loop
and to MCP servers that ask for it.

---

## 16. Agent mode (tool use)

The agent loop is exposed over RPC (`agent/run`) and is used internally
by the inline-edit and "diagnose test" surfaces. In 0.2.1 there is **no**
direct user entry point in the chat view yet — that arrives in 0.3.

When wired, the loop runs **up to `foundryCopilot.agent.maxSteps`** rounds of:

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

## 17. MCP servers (optional)

You can wire any number of stdio Model Context Protocol servers and
their tools become available to the agent loop automatically.

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

## 18. Standalone harness (no VS Code)

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

## 19. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| *"Endpoint must be HTTPS and end in a Microsoft Foundry domain"* | Your URL doesn't match the allow-list — check the host suffix |
| *"endpoint not configured"* in the output channel | `foundryCopilot.endpoint` is empty |
| *"DefaultAzureCredential: failed to acquire token"* | Run `az login`, or set `AZURE_TENANT_ID` / `AZURE_CLIENT_ID` / `AZURE_CLIENT_SECRET` env vars |
| *"deployment not found"* | The chat / completion / embedding deployment name doesn't exist in your Foundry resource |
| *"host not in foundry allow-list"* | The sidecar rejected an outbound URL — this is the lock doing its job; do **not** try to disable it |
| Activity-bar shield icon (`Foundry Copilot`) doesn't appear | Extension failed to activate — open Output → **Foundry Copilot** |
| Pressing `⌘⌥I` opens VS Code's built-in chat instead of Foundry | A higher-priority keybinding owns it; rebind via **File → Preferences → Keyboard Shortcuts** — search `foundryCopilot.chat.focus` |
| Status bar still shows ⚠️ **BAA: conflict** | Run **Foundry Copilot: BAA — Disable Conflicting Extensions Now**, or reload after the auto-disable |
| Status bar shows 🛡️ **BAA: off** (red) | `foundryCopilot.baa.enforcement` is `off` — set to `auto` to re-enable the boundary |
| Built-in chat command-centre still shows in title bar | Another extension re-enabled it; run **Foundry Copilot: BYO Chat — Re-enforce Settings**, or set `chat.commandCenter.enabled` to `false` globally |
| Sidecar keeps restarting | Run **Foundry Copilot: Show Output Channel** for the panic; binary mismatch (wrong platform VSIX) is the most common cause |
| Inline completions never show | `foundryCopilot.completionDeployment` is unset, or the model doesn't support fast completions |
| Telemetry / Billing view is empty | `foundryCopilot.telemetry.localStore` is `false`, or you haven't made any Foundry calls yet |
| OTLP exporter refuses to send | Endpoint host isn't (dot-anchored) in `foundryCopilot.telemetry.otlpAllowedHosts` |
| Budget alert never fires | `foundryCopilot.billing.dailyBudgetUsd` is `0` (disabled) |

For deeper debugging set `foundryCopilot.logLevel` to `debug` and
restart the window.

---

## 20. Uninstall

```bash
code --uninstall-extension patmeh1.foundry-copilot
rm -rf ~/.foundry-copilot   # optional: removes cached index + config
```

---

## 21. License & security

MIT — see [LICENSE](../LICENSE). The hard-lock guarantees and threat
model live in [SECURITY.md](../SECURITY.md). The non-negotiable core
tenants live in the [README](../README.md#core-tenants).
