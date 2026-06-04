# Foundry Copilot v0.2 — Test Matrix

This matrix tracks every verification listed in [plan2.md](../plan2.md) plus
**negative** test cases that must fail (security regressions). Each row maps
to one CI job and one test ID in [matrix.csv](matrix.csv).

Legend:

- **Layer**: `go` = Go unit test, `go-int` = Go integration (FOUNDRY_LIVE=1),
  `ts` = TypeScript unit, `e2e` = `@vscode/test-electron`, `manual` = manual
  smoke.
- **Type**: `+` = success path, `-` = negative / must-reject.
- **Status**: `pass`, `fail`, `pending`. Initial v0.2 cut is mostly `pending`
  until the scaffolded code is fleshed out.

## Phase 11 — Foundations & BAA

| ID    | Layer    | Type | Description                                                                                  | Expected outcome                                  | Status   |
| ----- | -------- | ---- | -------------------------------------------------------------------------------------------- | ------------------------------------------------- | -------- |
| 11.01 | ts       | +    | BAA guard returns conflict list when GitHub.copilot is installed + enabled                   | Returns `["GitHub.copilot"]`                      | pending  |
| 11.02 | ts       | +    | BAA guard returns conflict list when GitHub.copilot-chat is installed + enabled              | Returns `["GitHub.copilot-chat"]`                 | pending  |
| 11.03 | ts       | +    | BAA guard returns empty list when no Copilot extensions present                              | Returns `[]`                                      | pending  |
| 11.04 | ts       | +    | BAA guard auto-mode invokes `disableExtension` once per id                                   | One call per conflict                             | pending  |
| 11.05 | ts       | +    | BAA guard prompt-mode emits exactly one modal                                                | `showWarningMessage` called once                  | pending  |
| 11.06 | ts       | -    | BAA guard off-mode does NOT call `disableExtension`                                          | Zero calls                                        | pending  |
| 11.07 | e2e      | +    | Install stub `GitHub.copilot` → activate → status bar amber → auto-disable → green within 1s | Status sequence amber→green                       | pending  |
| 11.08 | e2e      | +    | Reactivate with stub still enabled → amber → auto-disable → green                            | Re-enforce on every activation                    | pending  |
| 11.09 | e2e      | -    | Off-mode + stub installed → status bar stays red                                             | Red persists                                      | pending  |
| 11.10 | go       | +    | Control-plane lock allows `management.azure.com`                                             | `ValidateEndpoint` returns nil                    | pending  |
| 11.11 | go       | -    | Control-plane lock rejects `api.openai.com`                                                  | Returns `ErrEndpointNotControlPlane`              | pending  |
| 11.12 | go       | -    | Control-plane lock rejects `management.azure.com.attacker.com`                               | Rejected (dot-anchoring)                          | pending  |
| 11.13 | go       | -    | Control-plane lock rejects http (non-https)                                                  | Rejected                                          | pending  |
| 11.14 | go       | -    | Control-plane lock rejects IP literal                                                        | Rejected                                          | pending  |
| 11.15 | ts       | +    | Supervisor restarts sidecar with 1s backoff after crash                                      | Restart at 1s mark                                | pending  |
| 11.16 | ts       | +    | Supervisor backoff doubles: 1→2→4→8→16→30                                                    | Sequence matches                                  | pending  |
| 11.17 | ts       | +    | Circuit opens after 5 failures in 60s                                                        | No restart attempt in next 60s                    | pending  |
| 11.18 | ts       | +    | Status bar shows green when sidecar healthy                                                  | Item background = green theme color               | pending  |
| 11.19 | ts       | +    | Status bar shows yellow during restart attempts                                              | Item background = warning                         | pending  |
| 11.20 | ts       | +    | Status bar shows red when circuit open                                                       | Item background = error                           | pending  |
| 11.21 | e2e      | +    | Embedding auto-discovery seeds `embeddingDeployment` when unset                              | Setting populated post-activation                 | pending  |
| 11.22 | e2e      | -    | Embedding auto-discovery does NOT overwrite explicit setting                                 | Setting unchanged                                 | pending  |
| 11.23 | e2e      | +    | `vscode.lm.selectChatModels({vendor:"foundry"})` returns one entry per deployment            | Length equals deployment count                    | pending  |
| 11.24 | e2e      | +    | LM provider prompt streams via `chat/stream` and logs telemetry row                          | Row count +1 after prompt                         | pending  |
| 11.25 | e2e      | +    | `agent.mode=foundry-agents` routes to `*.services.ai.azure.com` REST API                     | URL matches                                       | pending  |

## Phase 12 — Copilot Chat UI replacement

| ID    | Layer  | Type | Description                                                                                | Expected outcome                                  | Status  |
| ----- | ------ | ---- | ------------------------------------------------------------------------------------------ | ------------------------------------------------- | ------- |
| 12.01 | e2e    | +    | Open view container → "New thread" → stream renders                                        | Webview visible, response appears                 | pending |
| 12.02 | e2e    | +    | Thread persists across window reloads                                                      | Restored on activation                            | pending |
| 12.03 | e2e    | +    | Mode toggle Ask → no tools in prompt envelope                                              | Tool list empty                                   | pending |
| 12.04 | e2e    | +    | Mode toggle Edit → emits proposed-edits + opens diff preview                               | Diff pane visible                                 | pending |
| 12.05 | e2e    | +    | Mode toggle Agent → tool-call cards render                                                 | Card per tool invocation                          | pending |
| 12.06 | e2e    | +    | `@foundry` selectable from `@` menu                                                        | Item in picker                                    | pending |
| 12.07 | e2e    | +    | `/explain` selectable from `/` menu                                                        | Item in picker                                    | pending |
| 12.08 | e2e    | +    | `#file:src/foo.ts` resolved to file content in chat payload                                | Content present in last message                   | pending |
| 12.09 | e2e    | +    | Attachment chip: drag .ts file → chip appears → sent in payload                            | Chip renders, payload has attachment              | pending |
| 12.10 | e2e    | +    | Code block "Apply in editor" produces `WorkspaceEdit` → editor reflects change             | Document text updated                             | pending |
| 12.11 | e2e    | +    | Multi-file diff preview: Keep per file → only that file applied                            | One file modified                                 | pending |
| 12.12 | e2e    | +    | Multi-file diff preview: Discard all → no files modified                                   | Workspace unchanged                               | pending |
| 12.13 | e2e    | +    | `Cmd+I` in editor opens inline popover at cursor                                           | Popover visible                                   | pending |
| 12.14 | e2e    | +    | Inline chat accept inserts via `WorkspaceEdit`; one-step undo reverts                      | Undo restores original                            | pending |
| 12.15 | e2e    | +    | `Ctrl+Shift+I` opens quick chat globally                                                   | Floating window                                   | pending |
| 12.16 | e2e    | +    | Quick chat close → no thread persisted                                                     | Thread list unchanged                             | pending |
| 12.17 | e2e    | +    | Terminal `Cmd+I` → proposed command in confirm modal → accept runs                         | Command executed                                  | pending |
| 12.18 | e2e    | +    | Notebook cell right-click → "Foundry: Fix this cell" → diff preview                        | Diff visible with cell content                    | pending |
| 12.19 | go     | +    | `chat/edit_propose` RPC accepts messages and returns structured edits                      | `{files: [...]}` shape                            | pending |
| 12.20 | go     | +    | `chat/variables_list` returns 8 built-in variable names                                    | Array length 8                                    | pending |
| 12.21 | go     | +    | `chat/variables_resolve` returns content for `#file` request                               | Resolved string                                   | pending |

## Phase 13 — Copilot feature parity

| ID    | Layer  | Type | Description                                                                          | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ------------------------------------------------------------------------------------ | ------------------------------------------------ | ------- |
| 13.01 | go     | +    | `propose_edits` tool emits `{file, range, replacement, rationale}` records           | Schema matches                                   | pending |
| 13.02 | e2e    | +    | Edit mode → propose_edits → diff preview shows all files                             | All proposed files visible                       | pending |
| 13.03 | go     | +    | NES predictor returns `{file, range, text}` for known signature change               | Non-empty response                               | pending |
| 13.04 | e2e    | +    | NES off by default → no ghost text appears                                           | No suggestion                                    | pending |
| 13.05 | e2e    | +    | NES opt-in: edit signature → suggestion at call site within 800ms                    | Suggestion visible                               | pending |
| 13.06 | e2e    | +    | Smart action "Foundry: Explain" available in editor context menu                     | Item present                                     | pending |
| 13.07 | e2e    | +    | SCM "Generate" button reads staged diff → fills commit input                         | Input box populated                              | pending |
| 13.08 | e2e    | +    | Generated commit message follows Conventional Commits                                | Matches `/^(feat\|fix\|chore)/`                  | pending |
| 13.09 | e2e    | +    | PR description "Generate with Foundry" populates description field                   | Field non-empty                                  | pending |
| 13.10 | e2e    | +    | Test Explorer "Foundry: Diagnose failure" opens chat with test source + error        | Thread visible with context                      | pending |
| 13.11 | e2e    | +    | `.foundry/instructions/01-style.md` prepended to system prompt                       | First system msg contains file                   | pending |
| 13.12 | e2e    | +    | One-time migration prompt fires when `.github/copilot-instructions.md` present       | Prompt shown once                                | pending |
| 13.13 | e2e    | -    | Migration prompt dismissed → never fires again                                       | Suppressed across reloads                        | pending |
| 13.14 | go     | +    | Chat variable provider `#file` returns file content                                  | Bytes match                                      | pending |
| 13.15 | go     | +    | Chat variable provider `#codebase` calls RAG search                                  | RAG hit count > 0                                | pending |
| 13.16 | e2e    | +    | "Foundry: Fix this diagnostic" code action appears for any diagnostic                | Action item present                              | pending |
| 13.17 | e2e    | -    | Image attach button disabled when selected model lacks `vision`                      | Button disabled, tooltip explains                | pending |
| 13.18 | e2e    | +    | Image attach enabled with vision-capable model                                       | Button enabled                                   | pending |
| 13.19 | e2e    | +    | `.foundry/prompts/refactor.md` appears as `/refactor` slash command                  | Item in `/` picker                               | pending |

## Phase 14 — Deployment workflows

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 14.01 | go     | +    | `deploy/list` returns deployments with `{name, model, sku, capacity, state}` | Shape matches schema                             | pending |
| 14.02 | go     | -    | `deploy/list` returns error when endpoint unset                              | Error                                            | pending |
| 14.03 | go-int | +    | `deploy/create` then `deploy/get` returns the created deployment             | Get succeeds                                     | pending |
| 14.04 | go-int | +    | `deploy/delete` removes the deployment                                       | Subsequent get 404                               | pending |
| 14.05 | go-int | +    | `deploy/test` returns OK for a healthy deployment                            | `{ok: true}`                                     | pending |
| 14.06 | e2e    | +    | Tree view "Foundry Deployments" lists deployments                            | Tree populated                                   | pending |
| 14.07 | e2e    | +    | `foundryCopilot.createDeployment` runs quick-pick → progress → refresh       | Tree refreshed                                   | pending |
| 14.08 | e2e    | +    | Right-click "Open in Azure Portal" launches browser to correct resource URL  | URL matches                                      | pending |

## Phase 15 — Telemetry dashboards

| ID    | Layer  | Type | Description                                                                            | Expected outcome                                 | Status  |
| ----- | ------ | ---- | -------------------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 15.01 | go     | +    | Telemetry store opens SQLite file under `~/.foundry-copilot/telemetry.db`              | File created                                    | pending |
| 15.02 | go     | +    | Single chat call appends exactly one row                                               | Row count +1                                    | pending |
| 15.03 | go     | +    | `telemetry/summary` returns aggregates over 24h window                                 | Non-empty result                                | pending |
| 15.04 | go     | +    | `telemetry/timeseries` returns hourly buckets                                          | 24-element array                                | pending |
| 15.05 | go     | -    | OTLP export is no-op when `allowedOtlpHosts` empty                                     | Zero outbound requests                          | pending |
| 15.06 | go     | -    | OTLP export rejects host not in `allowedOtlpHosts`                                     | Error                                           | pending |
| 15.07 | e2e    | +    | Insights webview renders 4 charts after seeding rows                                   | 4 chart elements                                | pending |
| 15.08 | e2e    | +    | Status bar shows today's token total                                                   | Numeric text                                    | pending |
| 15.09 | e2e    | +    | CSV export downloads file with correct header row                                      | First row matches schema                        | pending |

## Phase 16 — Fine-tuning

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 16.01 | go     | +    | `dataset/from_sessions` produces JSONL passing validator                     | Validator returns ok                            | pending |
| 16.02 | go     | +    | `dataset/upload` posts multipart body with correct file field                | Mock server sees `file`                         | pending |
| 16.03 | go     | +    | `finetune/create_job` posts expected JSON                                    | Mock receives `{model, training_file}`          | pending |
| 16.04 | go     | +    | `finetune/get_job` returns parsed status                                     | Status field present                            | pending |
| 16.05 | go     | +    | `finetune/cancel_job` posts to /cancel endpoint                              | Mock sees cancel call                           | pending |
| 16.06 | e2e    | +    | Fine-tuning webview lists datasets and jobs                                  | Both lists rendered                             | pending |
| 16.07 | e2e    | +    | "Register fine-tuned deployment" launches Phase-14 create flow               | Create deployment dialog opens                  | pending |

## Phase 17 — Team features

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 17.01 | go     | +    | `.foundry/policy.yaml` allow-list parsed                                     | Struct populated                                | pending |
| 17.02 | go     | -    | `chat/start` with disallowed deployment returns policy violation             | `policy_violation` error                        | pending |
| 17.03 | go     | -    | `agent/run` calling disallowed tool returns policy violation                 | `policy_violation` error                        | pending |
| 17.04 | go     | +    | `.foundry/tools/*.json` adds tools to registry                               | Registry length +N                              | pending |
| 17.05 | go     | +    | `rag/attach_remote` verifies ed25519 signature                               | Pass with valid sig                             | pending |
| 17.06 | go     | -    | `rag/attach_remote` rejects tampered manifest                                | Signature error                                 | pending |
| 17.07 | go     | -    | `rag/attach_remote` rejects unsigned manifest                                | Error                                           | pending |
| 17.08 | e2e    | +    | `.foundry/prompts/refactor.md` appears as `/refactor`                        | (also tested in 13.19)                          | pending |

## Phase 18 — Billing

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 18.01 | go     | +    | Quota header parser extracts `x-ratelimit-remaining` from response           | Value parsed as int                             | pending |
| 18.02 | go     | -    | Quota parser tolerates missing headers                                       | Zero, no panic                                  | pending |
| 18.03 | go-int | +    | `billing/summary --days 7` returns non-empty cost rows                       | Rows > 0                                        | pending |
| 18.04 | go     | +    | Budget alert fires exactly once per day when threshold crossed               | One notification                                | pending |
| 18.05 | go     | -    | Budget alert does not double-fire on same day                                | Idempotent                                      | pending |
| 18.06 | e2e    | +    | Billing tab in Insights renders last-7-day chart                             | Chart visible                                   | pending |

## Phase 19 — Release

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| 19.01 | manual | +    | `git tag v0.2.0-rc.1` triggers release workflow → 5 VSIXes published         | Release contains 5 assets                       | pending |
| 19.02 | manual | +    | Install darwin-arm64 VSIX → smoke chat works                                 | Chat response visible                           | pending |
| 19.03 | manual | +    | Install linux-x64 VSIX → smoke inline completion works                       | Completion appears                              | pending |
| 19.04 | manual | +    | Install win32-x64 VSIX → smoke BAA disables stub Copilot                     | Status bar green                                | pending |

## Security & lock regressions (must NEVER pass)

These exist to make sure we never quietly relax security guarantees.

| ID    | Layer  | Type | Description                                                                  | Expected outcome                                 | Status  |
| ----- | ------ | ---- | ---------------------------------------------------------------------------- | ------------------------------------------------ | ------- |
| S.01  | go     | -    | Data-plane lock allows `api.openai.com` → must reject                        | Rejected                                        | pending |
| S.02  | go     | -    | Data-plane lock allows `.openai.azure.com.evil.com` → must reject            | Rejected (dot-anchor)                           | pending |
| S.03  | go     | -    | Data-plane lock allows `localhost` → must reject                             | Rejected                                        | pending |
| S.04  | go     | -    | Data-plane lock allows IP literal → must reject                              | Rejected                                        | pending |
| S.05  | go     | -    | Data-plane lock allows http:// → must reject                                 | Rejected                                        | pending |
| S.06  | go     | -    | Control-plane lock allows `api.openai.com` → must reject                     | Rejected                                        | pending |
| S.07  | go     | -    | Control-plane lock allows `management.azure.com.attacker.com` → must reject  | Rejected (dot-anchor)                           | pending |
| S.08  | go     | -    | Auth path that reads `OPENAI_API_KEY` env var → must NOT exist               | Code grep returns zero                          | pending |
| S.09  | ts     | -    | BAA guard with `off` mode and Copilot installed → status bar RED, never green| Red persists                                    | pending |
| S.10  | go     | -    | Telemetry export to non-allow-listed host → must reject                      | Error                                           | pending |
| S.11  | ts     | -    | Endpoint setting matches `.openai.com` (not Foundry) → settings UI rejects   | UI validation fails                             | pending |
| S.12  | go     | -    | Issuer verification with Google STS-issued token → must reject               | `ErrUntrustedIssuer`                            | pending |

## Summary by phase

| Phase | Positive | Negative | Total |
| ----- | -------- | -------- | ----- |
| 11    | 22       | 3        | 25    |
| 12    | 21       | 0        | 21    |
| 13    | 17       | 2        | 19    |
| 14    | 7        | 1        | 8     |
| 15    | 7        | 2        | 9     |
| 16    | 7        | 0        | 7     |
| 17    | 4        | 4        | 8     |
| 18    | 4        | 2        | 6     |
| 19    | 4        | 0        | 4     |
| S     | 0        | 12       | 12    |
| **Sum** | **93** | **26**   | **119** |
