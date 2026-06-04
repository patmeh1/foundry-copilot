# Plan: Foundry-Locked Copilot Clone — v0.2

Builds on v0.1 (the 10-phase plan archived in [plan.md](plan.md)). v0.1 shipped
the IDE-side feature set (chat / completions / agent / RAG / MCP) and the
network hard-lock. v0.2 turns the extension from a **caller** of Foundry into
an **operator** of Foundry: users manage the deployments and fine-tunes that
drive the lock, see exactly what they spend, and let a team share
prompts/tools/policy without giving up local-loop control.

## Core premise: BAA boundary (non-negotiable)

GitHub Copilot and GitHub Copilot Chat ship **without** a Business Associate
Agreement (BAA) covering protected-data workloads. Microsoft Foundry, by
contrast, **does** sit under standard Azure enterprise BAA terms. This
extension's *whole reason for existing* is to make that boundary the default:
every model token leaves the IDE through Foundry, and no token leaves through
Copilot.

Consequence:

- The extension **must** detect any `GitHub.copilot*` extension on activation
  and on every `vscode.extensions.onDidChange` event.
- The extension **must** offer a one-click "Disable conflicting Copilot
  extensions" action, and — when the user opts into auto-enforcement —
  invoke `workbench.extensions.disableExtension` for each conflicting id at
  workspace scope.
- The extension **must** surface the BAA enforcement state continuously via
  a dedicated status-bar item (green shield = enforced, amber = conflict
  detected, red = user-overridden).
- The extension **must** document this stance prominently in
  [docs/USER_GUIDE.md](docs/USER_GUIDE.md) and in the activation welcome
  walkthrough.

This is the **second** lock layer (alongside the network hard-lock from v0.1)
and is treated with the same rigor: dedicated package, dedicated tests,
dedicated owner in the architecture, and never relaxed by a "for development"
override.

## TL;DR

Nine themes — one foundations phase (BAA + cross-cutting), two
**Copilot-replacement** phases that close the UX gap created by disabling
GitHub Copilot Chat, and six operator-features phases:

- **Phase 11 — Foundations & cross-cutting**: **BAA enforcer**, control-plane
  HTTP client, `LanguageModelChatProvider`, sidecar supervisor (backoff +
  circuit breaker + status bar), embedding auto-discovery, Foundry Agents
  Service toggle. Resolves the BAA premise plus all four
  Further-Consideration decisions in one phase.
- **Phase 12 — Copilot Chat UI replacement** *(MUST land alongside Phase 11
  because Phase 11 disables Copilot Chat)*: full Foundry Chat view container
  with thread list, mode toggle (Ask / Edit / Agent), model picker,
  attachment chips (file / selection / image / symbol), `#`/`@`/`/` menus,
  streaming render with apply-in-editor / insert / run-in-terminal action
  buttons, multi-file diff preview with Keep/Discard, thread history, inline
  chat (Cmd+I), quick chat (Ctrl+Shift+I), terminal chat, notebook-aware chat.
- **Phase 13 — Copilot feature parity (Foundry-only)** *(also gated to
  v0.2.2)*: edit mode (multi-file edits), Next Edit Suggestions, smart
  actions (right-click Explain / Refactor / Generate tests / Fix),
  commit-message + PR-description generation, test-failure diagnose, custom
  instructions (`.foundry/instructions/*.md` + `.github/copilot-instructions.md`
  fallback), chat-variable providers (`#file`, `#selection`, `#editor`,
  `#terminalLastCommand`, `#codebase`, `#problems`, `#folder`), diagnostic
  code actions, voice input (model-capability gated), vision attachments
  (model-capability gated).
- **Phase 14 — Model deployment workflows**: list / create / scale / delete
  Foundry deployments from inside VS Code; quick-test pad.
- **Phase 15 — Telemetry dashboards**: instrument every Foundry call; local
  SQLite store; "Foundry Insights" webview; local-first, opt-in OTLP export.
- **Phase 16 — Fine-tuning workflows**: dataset from sessions → upload →
  submit fine-tune → monitor → register deployment.
- **Phase 17 — Team features**: checked-in `.foundry/` (prompts, tools,
  policy, MCP catalog); optional signed shared RAG index.
- **Phase 18 — In-extension billing**: Cost Management surface + quota
  headers; budget alerts; status-bar indicator.
- **Phase 19 — v0.2 release**: rolling minor releases (`0.2.0` → `0.2.8`)
  per completed phase; consolidated user guide.

The hard lock is **extended**, never relaxed: a new `core/internal/control`
package introduces a second `LockedTransport` for `management.azure.com`
(control plane), and a new `extension/src/baa/` module enforces the BAA
boundary by disabling conflicting Copilot extensions. v0.1's data-plane
`LockedTransport` is untouched.

## Phasing

### Phase 11 — Foundations & cross-cutting *(blocks 12, 13, 14, 16, 18; partial-blocks 15, 17)*

1. **BAA enforcer** (`extension/src/baa/guard.ts` + status-bar
   `extension/src/baa/status.ts`).
   - Default conflict list (settable via
     `foundryCopilot.baa.conflictingExtensions`, default
     `["GitHub.copilot", "GitHub.copilot-chat", "GitHub.copilot-workspace", "GitHub.copilot-labs"]`).
   - On activation and on `vscode.extensions.onDidChange`, enumerate
     `vscode.extensions.all` and flag any installed-and-enabled match.
   - Two enforcement modes via `foundryCopilot.baa.enforcement`: `"auto"`
     (default) — invoke `workbench.extensions.disableExtension` for each
     conflicting id at the configured `vscode.ConfigurationTarget` (default
     `Workspace`); `"prompt"` — show a modal listing the conflicts with
     one-click "Disable all (workspace)" and "Disable all (globally)"
     actions; `"off"` — log a warning only, status bar goes red.
   - Status-bar item `Foundry BAA` on the left, three states:
     🛡 green ("BAA enforced — Copilot disabled"), ⚠ amber
     ("Conflict detected — click to resolve"), ⛔ red ("BAA enforcement
     off — at-risk"). Tooltip lists which extensions are currently active.
   - Welcome walkthrough page (`extension/walkthroughs/baa.md`) registered
     via `contributes.walkthroughs`, shown once on first activation,
     explaining BAA reasoning + showing current state.
   - When auto-disable command fails (older VS Code without that internal
     command), fall back to `prompt` mode and emit a one-time notification.
2. **Control-plane HTTP client**. `core/internal/control` package with its
   own `LockedTransport` and `AllowedControlHosts = ["management.azure.com"]`,
   mirroring the v0.1 lock pattern. Reuses `DefaultAzureCredential` with ARM
   scope.
3. **Sidecar supervisor** in `extension/src/sidecar/process.ts`: exponential
   backoff (1→2→4→8→16→30s), circuit breaker (5 failures in 60s → open for
   60s), tri-color status-bar item. Failures stream to the existing output
   channel; no toasts.
4. **Embedding deployment auto-discovery** at activation; seeds
   `foundryCopilot.embeddingDeployment` only if unset. Explicit setting
   always wins.
5. **`LanguageModelChatProvider`** in `extension/src/lm/provider.ts`. Each
   chat deployment becomes a model "Foundry / {deployment}". Routes through
   existing `chat/stream`. New setting `foundryCopilot.lmProvider.enabled`
   (default `true`). **NOTE**: with BAA enforcer active, GitHub Copilot Chat
   is disabled, so this provider is consumed by other extensions calling
   `vscode.lm.selectChatModels({ vendor: "foundry" })` — not by Copilot
   Chat's UI. Our own `@foundry` participant + Foundry Chat view from Phase
   12 is the primary chat surface.
6. **Foundry Agents Service backend toggle**: setting `foundryCopilot.agent.mode`
   = `local` (default) | `foundry-agents`. New `core/internal/foundryagents`
   REST client; local loop in `core/internal/agent/loop.go` remains the
   default.

### Phase 12 — Copilot Chat UI replacement *(MUST land alongside Phase 11)*

> Disabling GitHub Copilot Chat (Phase 11) without an equivalent UI is a UX
> regression. This phase delivers a full Foundry-native chat experience that
> **replaces** what users lose. v0.1's `@foundry` participant + webview is
> the starting point; this phase upgrades it to Copilot-Chat-class.

1. **Foundry Chat view container** (replaces the v0.1 webview panel as the
   primary chat surface): activity-bar container `extension/src/views/chat/`
   with three sub-views — thread list, compose surface, run/diff inspector.
2. **Thread list sidebar**: persisted-session list (reuses v0.1 on-disk
   session store); rename / delete / duplicate / export-as-markdown
   context-menu items; "New thread" button; search box.
3. **Compose surface**: markdown editor with sticky toolbar. **Mode toggle**
   (Ask / Edit / Agent) drives a different system prompt + tool subset on
   the sidecar. **Model picker dropdown** populated from Foundry deployments
   (Phase 14 surface; falls back to settings until Phase 14 lands).
4. **Triggered menus**:
   - `@` → participant picker (`@foundry`, `@foundry-edit`, `@foundry-agent`,
     plus any registered MCP-backed participants).
   - `/` → slash menu (`/explain`, `/fix`, `/tests`, `/doc`, `/agent`, plus
     user-defined from `.foundry/prompts/*.md`).
   - `#` → chat-variable menu (`#file`, `#selection`, `#editor`,
     `#terminalLastCommand`, `#codebase`, `#problems`, `#folder`, `#symbol`).
     Resolution lives in Phase 13; UI lands here.
5. **Attachment chips**: file (single + multi via picker or drag-drop),
   selection, image (vision), code symbol. Drag-drop from Explorer / editor
   / terminal; right-click "Add to Foundry Chat" in Explorer and editor
   menus.
6. **Streaming render**: incremental markdown with syntax highlighting
   (`shiki`, already MIT/no native deps). Per code-block action bar —
   Copy / Insert at cursor / Apply in editor / Run in terminal / Save to
   file. Token meter in footer.
7. **Tool-call render**: expandable card showing tool name, input JSON,
   output (truncated with "show full") streaming as the agent runs.
8. **Multi-file diff preview pane** (used by Edit and Agent modes): when
   sidecar emits proposed file edits, render a tabbed diff (file-per-tab)
   with **Keep / Discard** per file and **Keep all / Discard all** for the
   whole change set; "Open in diff editor" opens VS Code's native diff for
   big patches.
9. **Inline chat** (`Cmd+I` in editor): popover compose anchored at the
   cursor, scoped to current selection or surrounding function
   (`vscode.languages.getDocumentSymbol`); accept/discard inline; results
   inserted with `WorkspaceEdit` so undo works.
10. **Quick chat** (`Ctrl+Shift+I` global): floating mini compose window
    with no thread persistence; one-shot ask routed through `chat/stream`.
11. **Terminal chat** (`Cmd+I` in integrated terminal): "Propose a command"
    surface; agent returns commands; "Insert & run" with confirm modal
    (reuses Phase 5 approval UI from v0.1).
12. **Notebook-aware chat**: register cell-scoped commands so right-clicking
    a notebook cell offers "Foundry: Ask about this cell" / "Foundry: Fix
    this cell" / "Foundry: Add tests for this cell". Context includes
    neighboring cells and kernel state when available.
13. **New RPCs** in `core/internal/rpc/server.go`: `chat/edit_propose`
    (returns `{files: [{path, before, after, range}]}`), `chat/edit_apply`,
    `chat/edit_discard`, `chat/variables_list`, `chat/variables_resolve`.

### Phase 13 — Copilot feature parity (Foundry-only) *(depends on Phase 12)*

> Closes the **functional** gap from disabling Copilot Chat. Each feature
> here only works because we route through Foundry — there's no fallback to
> Copilot.

1. **Edit mode** — multi-file edits proposed by the model; uses Phase 12's
   diff preview as the review surface. Sidecar tool `propose_edits` (new in
   `core/internal/tools/tool.go`) emits structured `{file, range,
   replacement, rationale}` records that the extension turns into a
   `WorkspaceEdit`.
2. **Next Edit Suggestions (NES)** — beyond inline completion, predict the
   next *edit location* + content. New `core/internal/nes` package; new RPC
   `nes/predict`; extension renders a cursor-jump affordance + ghost-text.
   Uses a small Foundry chat deployment (settings:
   `foundryCopilot.nes.deployment`, default falls back to `chatDeployment`).
   Off by default; opt-in via `foundryCopilot.nes.enabled`.
3. **Smart actions** — register `vscode.languages.registerCodeActionsProvider`
   and editor-context menu items: "Foundry: Explain", "Foundry: Refactor",
   "Foundry: Generate tests", "Foundry: Add docs", "Foundry: Fix this".
4. **Commit-message generation** — SCM input-box button: reads
   `git diff --cached`, sends to a chat deployment with the
   Conventional-Commits prompt, fills the input box.
5. **PR description generation** — when the GitHub PR extension is present,
   contribute an action on the PR title/description fields ("Generate with
   Foundry"). Pulls diff via `git diff main...HEAD`.
6. **Test failure diagnosis** — Test Explorer integration: failed test →
   context-menu "Foundry: Diagnose failure"; bundles the failing test
   source, error output, and traced source files into a single chat turn.
7. **Custom instructions** — auto-load `.foundry/instructions/*.md`
   (alphabetical order, prepended to system prompt). Detect
   `.github/copilot-instructions.md` once; if present, show a one-time
   prompt offering to copy/symlink it under `.foundry/instructions/`.
8. **Chat-variable resolution** — implement provider for each `#`-variable
   from Phase 12 step 4 in `core/internal/chatvars`; resolved values are
   injected as additional `tool` messages in the chat stream. `#codebase`
   queries the v0.1 RAG index.
9. **Diagnostic code actions** —
   `vscode.languages.registerCodeActionsProvider` for `CodeActionKind.QuickFix`
   on any diagnostic; offers "Foundry: Fix this diagnostic" which sends
   `{file, range, diagnostic.message, surrounding code}` to the chat in
   Edit mode and shows the diff preview.
10. **Voice input** *(model-capability gated)* — register a
    `vscode.SpeechProvider` adapter that routes microphone audio to a
    Foundry speech-to-text deployment. Setting
    `foundryCopilot.voice.deployment`. Off by default.
11. **Vision attachments** *(model-capability gated)* — image attachments
    from Phase 12 step 5 are only allowed when the selected model picker
    entry is flagged `vision`; the sidecar inspects deployment capabilities
    (Phase 14 list) and disables the image button otherwise.
12. **Custom slash commands** — `.foundry/prompts/*.md` files surface as
    slash commands.

### Phase 14 — Model deployment workflows *(depends on Phase 11)*

1. Sidecar deployment client in `core/internal/control/deployments.go`. List
   (filterable by capability), get, create (model, SKU, capacity),
   update-capacity, delete. Uses the new control-plane `LockedTransport`.
   Each deployment record carries `{name, model, sku, capacity, state, etag,
   capabilities}`.
2. New RPCs in `core/internal/rpc/server.go`: `deploy/list`, `deploy/get`,
   `deploy/create`, `deploy/update`, `deploy/delete`, `deploy/test` (sends
   a one-token ping to verify a deployment is reachable).
3. Typed RPC wrappers in `extension/src/sidecar/rpc.ts`.
4. Tree view "Foundry Deployments" in a new view container. Refresh, delete
   (with confirm modal), open-in-Azure-portal context-menu items.
5. Create deployment command `foundryCopilot.createDeployment`: quick-pick
   base model → input name → SKU/capacity webview → progress notification →
   tree-view refresh.
6. Test deployment command `foundryCopilot.testDeployment`: runs
   `deploy/test`.
7. Capabilities exposure: `deploy/list` results feed Phase 12's model
   picker AND Phase 13's vision/voice gating.

### Phase 15 — Telemetry dashboards *(parallel with 12–14)*

1. Wrap `(*foundry.Client).Chat` / `Embed` with a telemetry hook that
   captures `{deployment, prompt_tokens, completion_tokens, latency_ms,
   error}`. Local store in new `core/internal/telemetry` package using
   `modernc.org/sqlite` (pure-Go, no native deps).
2. RPCs: `telemetry/summary`, `telemetry/timeseries`, `telemetry/errors`,
   `telemetry/clear`.
3. Webview "Foundry Insights" in `extension/src/views/telemetry/` — four
   charts (tokens/hour, latency p50/p95, error rate, top deployments).
4. Settings: `foundryCopilot.telemetry.enabled` (default `true`),
   `foundryCopilot.telemetry.otlpEndpoint` (default empty),
   `foundryCopilot.telemetry.allowedOtlpHosts` (default empty).
5. Privacy doc section in user guide; "Where my data goes" walkthrough page.
6. Status-bar indicator showing today's spend (tokens × estimate).
7. Export to CSV command for compliance.

### Phase 16 — Fine-tuning workflows *(depends on 11 + 15)*

1. `dataset/from_sessions` builds a JSONL dataset from telemetry rows
   passing OpenAI fine-tune validator.
2. `dataset/upload` uploads to Foundry; `dataset/list`, `dataset/delete`.
3. `finetune/create_job`, `finetune/get_job`, `finetune/list_jobs`,
   `finetune/cancel_job`.
4. Webview "Foundry Fine-tuning" — dataset list, job list, "Register
   fine-tuned deployment" action.
5. Settings: `foundryCopilot.finetune.baseModel`,
   `foundryCopilot.finetune.dataset.dir`.
6. Doc: when fine-tuning is useful, dataset size guidance.

### Phase 17 — Team features *(parallel with 14/15)*

1. `.foundry/` directory in workspace root, checked in. Subdirs: `prompts/`,
   `tools/`, `policy.yaml`, `mcp.json`, `instructions/`, `variables/`.
2. Watcher reloads each on change; settings can pin specific subdirs.
3. `policy.yaml` — allowed deployments, agent tool allow-list, max tokens.
4. Tools overlay: `.foundry/tools/*.json` add to the agent tool registry
   (read-only HTTP/jq tools).
5. Shared RAG index — read-only signed manifest in customer Azure Blob;
   `rag/attach_remote`, `rag/detach_remote`, `rag/sync_remote`.
6. Per-workspace ed25519 signature verification on shared index manifests.

### Phase 18 — In-extension billing *(depends on 11 control plane + 15)*

1. Control-plane client for Cost Management API (subscription-scope queries).
2. Quota-header parser on every Foundry response (`x-ratelimit-*`).
3. RPCs: `billing/summary`, `billing/quota`, `billing/budget_set`,
   `billing/budget_get`.
4. Webview tab inside Insights with last-7-day cost line + remaining quota.
5. Budget alerts: when daily spend crosses a threshold, fire one
   notification per day (idempotent).
6. Settings: `foundryCopilot.billing.subscriptionId`,
   `foundryCopilot.billing.budget.dailyUsd`.

### Phase 19 — v0.2 release *(rolling minor releases gated per-phase)*

1. **Release cadence**: tagged minor per completed phase — `v0.2.0` after
   Phase 11+12+13 (paired ship), `v0.2.1` after Phase 14, etc., through
   `v0.2.5` for Phase 18 + 19 final. Reuses
   [.github/workflows/release.yml](.github/workflows/release.yml) and
   [scripts/package-all.sh](scripts/package-all.sh) from v0.1.
2. **Per-release**: bump `extension/package.json` version, run `npm install`
   to refresh `package-lock.json`, `git tag vX.Y.Z`, push.
3. **[docs/USER_GUIDE.md](docs/USER_GUIDE.md) addendum sections** per phase:
   "BAA boundary", "Foundry Chat (replaces Copilot Chat)", "Inline chat /
   Quick chat / Terminal chat", "Edit mode & Next Edit Suggestions",
   "Smart actions & commit messages", "Managing Deployments", "Telemetry &
   Privacy", "Fine-tuning", "Team configuration (.foundry/)", "Billing".
4. **`v0.2` final**: combined release-notes entry once all phases land;
   major-section refactor of the user guide TOC.

## Relevant files

### v0.1 files (unchanged or lightly touched)

- [core/internal/foundry/lock.go](core/internal/foundry/lock.go) — pattern
  to copy for `core/internal/control/lock.go`; **do not** add control-plane
  hosts here.
- [core/internal/foundry/auth.go](core/internal/foundry/auth.go) —
  `DefaultAzureCredential` reused; new ARM-scope `GetToken` call lives in
  the control package.
- [core/internal/foundry/lock_test.go](core/internal/foundry/lock_test.go)
  — table-driven test template for control-plane lock tests.
- [core/internal/foundry/client.go](core/internal/foundry/client.go) —
  `(*Client).Chat` / `(*Client).Embed` get telemetry-hook wrappers;
  ratelimit-header parsing for billing.
- [core/internal/agent/loop.go](core/internal/agent/loop.go) — agent-mode
  toggle hooks here; tool-call counting feeds telemetry.
- [core/internal/rpc/server.go](core/internal/rpc/server.go) — register
  new method families: `chat/edit_propose`, `chat/edit_apply`,
  `chat/edit_discard`, `chat/variables_list`, `chat/variables_resolve`,
  `nes/predict`, `deploy/*`, `telemetry/*`, `finetune/*`, `dataset/*`,
  `billing/*`, `policy/*`.
- [core/internal/tools/tool.go](core/internal/tools/tool.go) — add new
  `propose_edits` tool used by Edit / Agent modes; `Registry` extension
  point also feeds `.foundry/tools/*.json` overlays.
- [core/internal/rag/rag.go](core/internal/rag/rag.go) — companion
  `remote.go` for read-only shared index; existing on-disk format
  unchanged.
- [extension/src/sidecar/process.ts](extension/src/sidecar/process.ts) —
  replace spawn logic with supervisor; new status-bar tri-color item.
- [extension/src/sidecar/rpc.ts](extension/src/sidecar/rpc.ts) — typed
  wrappers for every new RPC family.
- [extension/src/extension.ts](extension/src/extension.ts) — register BAA
  guard FIRST (before sidecar spawn), then tree views, language-model
  provider, status-bar items, commands.
- [extension/src/chat/slash.ts](extension/src/chat/slash.ts) — load
  `.foundry/prompts/*.md` as additional slash commands.
- [extension/package.json](extension/package.json) — new `viewsContainers`,
  `views`, `commands`, `configuration` keys (incl. `foundryCopilot.baa.*`);
  declare `languageModelChatProviders` contribution; declare
  `contributes.walkthroughs`.
- [.github/workflows/release.yml](.github/workflows/release.yml) —
  unchanged; reused per minor release.
- [scripts/package-all.sh](scripts/package-all.sh) — unchanged; reused per
  minor release.
- [docs/USER_GUIDE.md](docs/USER_GUIDE.md) — addendum sections per phase,
  **including a top-level "BAA boundary" section above all existing content**.

### New v0.2 files

- `extension/src/baa/guard.ts` — detection + auto-disable logic; pure-TS,
  unit-testable with mocked `vscode.extensions`.
- `extension/src/baa/status.ts` — status-bar item + command bindings
  (`foundryCopilot.baa.recheck`, `foundryCopilot.baa.disableConflicts`,
  `foundryCopilot.baa.showWalkthrough`).
- `extension/walkthroughs/baa.md` — first-activation walkthrough explaining
  the BAA premise.
- `extension/src/views/chat/` — Foundry Chat view container; thread list,
  compose surface, run/diff inspector; the v0.1 webview panel is
  **superseded** by this and removed.
- `extension/src/views/chat/diff.ts` — multi-file diff preview pane +
  Keep/Discard wiring with `WorkspaceEdit`.
- `extension/src/inline/inline-chat.ts` — `Cmd+I` editor inline-chat
  popover.
- `extension/src/inline/quick-chat.ts` — `Ctrl+Shift+I` floating
  quick-chat.
- `extension/src/inline/terminal-chat.ts` — `Cmd+I` terminal command
  proposal.
- `extension/src/notebook/notebook-chat.ts` — cell-scoped notebook
  commands.
- `extension/src/nes/provider.ts` — Next Edit Suggestions provider wiring
  to `nes/predict` RPC.
- `extension/src/scm/commit-message.ts` — SCM input-box action for commit
  message generation.
- `extension/src/scm/pr-description.ts` — GitHub PR extension integration.
- `extension/src/tests/diagnose.ts` — Test Explorer integration for
  failure diagnosis.
- `extension/src/lm/provider.ts` — `LanguageModelChatProvider`
  registration.
- `extension/src/views/deployments.ts` — Phase 14 tree view.
- `extension/src/views/telemetry.ts` — Phase 15 Insights webview.
- `extension/src/views/finetune.ts` — Phase 16 fine-tuning webview.
- `extension/src/views/billing.ts` — Phase 18 billing tab.
- `extension/src/team/loader.ts` — `.foundry/` watcher + reloader.
- `core/internal/control/` — control-plane lock + deployments client +
  cost management.
- `core/internal/chatvars/` — providers for `#file`, `#selection`,
  `#editor`, `#terminalLastCommand`, `#codebase`, `#problems`, `#folder`,
  `#symbol`.
- `core/internal/nes/` — Next Edit Suggestions predictor; calls Foundry
  chat deployment with a structured prompt.
- `core/internal/telemetry/` — pure-Go SQLite store + RPC handlers.
- `core/internal/finetune/` — dataset + fine-tune workflows.
- `core/internal/team/` — `.foundry/policy.yaml` + tools overlay loader.
- `core/internal/billing/` — Cost Management + quota header parsing.
- `core/internal/foundryagents/` — Foundry Agents Service REST client.

## Verification

1. **BAA guard unit tests**: with mocked `vscode.extensions.getExtension`
   returning each of `GitHub.copilot`, `GitHub.copilot-chat`, etc., guard
   returns the expected conflict list; auto-mode invokes
   `workbench.extensions.disableExtension` once per id; prompt-mode emits
   exactly one modal; off-mode emits exactly one warning log.
2. **BAA integration test** (`@vscode/test-electron`): install a stub
   extension declaring id `GitHub.copilot` → activate ours → assert status
   bar shows amber within 1s → assert auto-disable fires → assert status
   bar transitions to green → uninstall stub → status bar stays green.
3. **BAA reactivation test**: re-activate ours with the stub still enabled
   → status bar starts amber → auto-disable runs → green within 1s.
4. **Foundry Chat view smoke**: open view container → create thread → send
   "explain this code" with `#selection` → assert stream renders, code
   block has Apply/Insert/Copy buttons, Apply produces a `WorkspaceEdit`,
   telemetry row recorded.
5. **Mode toggle**: Ask mode disables tools; Edit mode emits proposed-edits
   structured records and opens the diff preview; Agent mode shows
   tool-call cards.
6. **Inline chat (`Cmd+I`)**: open file → select function → invoke →
   assert popover anchored at cursor → accept inserts via `WorkspaceEdit`;
   undo reverts in one step.
7. **Quick chat (`Ctrl+Shift+I`)**: invoke globally → floating window
   opens → response streams → close → no thread persisted.
8. **Terminal chat (`Cmd+I` in terminal)**: invoke → request "list running
   docker containers" → confirm modal appears with proposed command →
   accept runs in terminal.
9. **Notebook chat**: open `.ipynb` → right-click cell → "Foundry: Fix
   this cell" → diff preview shows new cell content; "Keep" applies it.
10. **Next Edit Suggestions**: with `nes.enabled = true`, edit a function
    signature → assert NES surfaces a follow-up edit suggestion at the
    call site within 800ms.
11. **Smart action — commit message**: stage two files → click SCM
    "Generate" → assert message follows Conventional Commits and
    references both files.
12. **PR description (with GitHub PR ext present)**: create PR draft →
    click "Generate with Foundry" → description field populates.
13. **Test failure diagnose**: run a failing test → right-click in Test
    Explorer → "Foundry: Diagnose failure" → chat thread opens with test
    source + error in context.
14. **Custom instructions**: add `.foundry/instructions/01-style.md` →
    send chat → assert instruction is prepended to system prompt; with
    `.github/copilot-instructions.md` present → one-time migration prompt
    fires; declining suppresses it forever.
15. **Chat variables**: invoke `#file:src/foo.ts #problems` → sidecar
    receives both resolved values as tool messages.
16. **Vision gating**: select non-vision deployment → image attach button
    is disabled; select vision deployment → enabled and round-trips.
17. **Control-plane lock unit tests** (mirrors v0.1): allow
    `management.azure.com`, reject `api.openai.com`, `localhost`,
    `management.azure.com.attacker.com`. Run via
    `go test -race ./internal/control/...`.
18. **Supervisor simulation test**: kill sidecar 5x in a row; status bar
    flips through green → yellow → red; circuit reopens after 60s; output
    channel records each restart.
19. **Embedding auto-discovery**: with `embeddingDeployment` unset and a
    populated Foundry account, activation logs the discovered deployment
    and `foundryCopilot.embeddingDeployment` reflects it; with the setting
    explicitly set, discovery does not overwrite.
20. **`LanguageModelChatProvider`**:
    `vscode.lm.selectChatModels({ vendor: "foundry" })` returns one entry
    per configured chat deployment; sending a prompt streams through
    `chat/stream` and shows up in the telemetry table.
21. **Foundry Agents Service toggle**: with `agent.mode = foundry-agents`
    set, `agent/run` creates and polls a Foundry Agents thread.
22. **Deployment CRUD**: integration test gated by `FOUNDRY_LIVE=1` —
    create a small embedding deployment, ping with `deploy/test`, delete;
    tree view reflects each step.
23. **Telemetry round-trip**: send one chat → assert exactly one row
    appended to `telemetry.db` with correct token counts and latency.
24. **Insights webview**: with seeded telemetry rows, dashboard renders
    all four charts and pulls `telemetry/summary` exactly once per open.
25. **Fine-tune dry-run**: `dataset/from_sessions` produces a JSONL file
    passing OpenAI's published validator; `finetune/create_job` against a
    mocked endpoint posts the expected multipart body.
26. **Policy enforcement**: with a `.foundry/policy.yaml` that allows only
    deployment `gpt-4o-mini`, attempting `chat/start` with `gpt-5` returns
    a structured policy-violation error.
27. **Custom slash command**: `.foundry/prompts/refactor.md` makes
    `@foundry /refactor` appear in slash autocomplete; invoking it injects
    the file content as system prompt.
28. **Shared RAG index**: serve a signed `manifest.json` + `chunks.gob`
    from a local Go test server; sidecar verifies signature, attaches
    index, search returns hits.
29. **Billing summary**: gated by `FOUNDRY_LIVE=1`; `billing/summary
    --days 7` returns a non-empty result; quota tracker increments after
    each chat call.
30. **Budget alert**: with `billing.budget.dailyUsd = 0.01`, sending one
    chat fires exactly one notification per day (idempotency tested by
    simulating two crosses).
31. **Release dry-run** per minor: tag `v0.2.0-rc.N` → release workflow
    uploads five VSIXes → install on macOS arm64 → smoke-test chat + new
    feature.

## Decisions

- **BAA boundary is enforced by code, not by convention.** A dedicated
  guard module (`extension/src/baa/`) runs on every activation and on every
  `vscode.extensions.onDidChange` event. Default mode is `auto` (silent
  disable at workspace scope). User can opt down to `prompt` or `off`, but
  the status bar continues to surface the state in red whenever `off` is
  active.
- **Disabling Copilot without replacing it is a regression** — Phases
  12 + 13 ship alongside Phase 11 (versioned `v0.2.0` together) so that
  the moment a user installs this extension, Foundry-equivalent UX is in
  place for chat, inline chat, edit mode, commit messages, smart actions,
  and chat variables. The v0.1 webview panel is **superseded** by the
  Phase 12 Foundry Chat view container and removed.
- **The `@foundry` chat participant + LM provider both remain**, but
  they're no longer the primary chat surface — they exist for compatibility
  with other extensions and for any user who pins them. The Foundry Chat
  view container is the new home.
- **All four user-specified Further-Consideration choices accepted**: (1)
  auto-discover embedding deployment, (2) ship both participant AND
  `LanguageModelChatProvider`, (3) optional Foundry Agents toggle (default
  local), (4) supervisor with backoff + circuit breaker + status indicator.
- **Hard lock is extended, not relaxed** — second `LockedTransport` covers
  `management.azure.com` in `core/internal/control`; the data-plane
  allow-list in [core/internal/foundry/lock.go](core/internal/foundry/lock.go)
  is untouched.
- **No API-key code path is ever introduced**, even for billing or control
  plane. `DefaultAzureCredential` with the ARM scope only.
- **Telemetry is local-first** (SQLite under `~/.foundry-copilot/`); OTLP
  export is off by default behind a separate `AllowedOtlpHosts` opt-in.
- **Team features are file-based** (`.foundry/` in the repo). No SaaS
  server, no central control plane. Optional shared index uses
  customer-owned Azure Blob.
- **Pure-Go deps only** to preserve single-binary distribution:
  `modernc.org/sqlite` (not `mattn/go-sqlite3`); `shiki` is the chat
  webview syntax highlighter (Wasm-based, no native deps).
- **`vsce` packaging + `release.yml` unchanged** from v0.1; v0.2 just
  bumps versions per phase.

**Out of scope for v0.2**:

- Non-Foundry providers (still excluded by design).
- Hosted multi-tenant SaaS for teams.
- Native non-Go telemetry backend (Prometheus exporter etc.).
- Mobile / web companion clients.
- LLM-driven cost-optimization recommendations.
- Negotiating a BAA for GitHub Copilot itself (out of our control; the
  boundary stands).
- **Copilot's public-code matching surface** — that's unique to GitHub's
  training-data filter and has no Foundry equivalent.
- **GitHub.com server-side features** (PR review on github.com, issue
  suggestions in the GH web UI) — extension scope only.
- **Cross-IDE parity** (JetBrains, Visual Studio, Neovim) — VS Code only
  for v0.2; harness CLI covers non-VS-Code users.

## Further Considerations

1. **Telemetry storage choice — sqlite vs jsonl.** A *(recommended)*:
   `modernc.org/sqlite` (pure-Go, supports query rollups in the sidecar
   without loading entire history). B: append-only JSONL (simpler, but
   full-scan on every dashboard load). C: both, with JSONL as the
   source-of-truth and SQLite as a derived index.
2. **`LanguageModelChatProvider` default state.** A *(recommended)*:
   registered + enabled by default; one setting
   (`foundryCopilot.lmProvider.enabled`) to opt out. B: registered but
   disabled by default. C: separate VSIX bundle.
3. **Shared RAG index distribution channel.** A *(recommended)*:
   customer-managed Azure Blob with a signed `manifest.json`. B: OCI
   artifact in a customer registry. C: git-LFS-tracked binary in the
   workspace repo.
4. **BAA enforcement default mode.** A *(recommended)*: `auto` at
   workspace scope. B: `auto` at global scope. C: `prompt` by default.
5. **BAA conflict scope.** A *(recommended)*: ship the default list
   `["GitHub.copilot", "GitHub.copilot-chat", "GitHub.copilot-workspace",
   "GitHub.copilot-labs"]`, setting overrides. B: empty default list. C:
   pull list from a remote manifest in our repo.
6. **Chat UI hosting surface.** A *(recommended)*: dedicated **activity-bar
   view container** ("Foundry Chat") with its own thread sidebar — the
   natural Copilot Chat replacement, prime real estate, supports multiple
   sub-views (threads + diff inspector). B: keep the v0.1
   webview-panel-in-editor-area as the primary surface. C: a
   status-bar-anchored slide-out — rejected.
7. **Edit-mode acceptance UI.** A *(recommended)*: per-file Keep / Discard
   in the diff preview pane + a global Keep All / Discard All in the
   toolbar — matches what Copilot Chat's Edits surface does. B: auto-apply,
   with an undo notification — faster but riskier. C: open VS Code's native
   multi-diff editor for every change — heavyweight.
8. **Next Edit Suggestions default state.** A *(recommended)*: **off by
   default** (opt-in via `foundryCopilot.nes.enabled`) — NES adds
   steady-state Foundry traffic and cost. B: on by default with a separate
   "NES deployment" setting required. C: ship without NES in v0.2.
9. **Chat variables — pluggability.** A *(recommended)*: ship the eight
   built-in variables implemented in `core/internal/chatvars`; expose
   `.foundry/variables/*.json` in a follow-up for user-defined ones. B:
   ship only `#file` and `#selection`. C: skip `#codebase` in v0.2.
10. **Voice + vision gating.** A *(recommended)*: hard-gate on deployment
    capability — image attach and microphone buttons are disabled (with a
    tooltip) when the selected deployment lacks the capability. B: always
    show the buttons; surface an error on click. C: hide the buttons
    entirely until a capable deployment is configured.
11. **Custom-instructions file format.** A *(recommended)*:
    alphabetical-by-filename merge of `.foundry/instructions/*.md`
    (Markdown), with one-time migration prompt for
    `.github/copilot-instructions.md`. B: a single
    `.foundry/instructions.md`. C: YAML front-matter on each file with
    priority field.
