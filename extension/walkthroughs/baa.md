# Foundry Copilot — BAA Boundary

GitHub Copilot and GitHub Copilot Chat ship without a Microsoft **Business
Associate Agreement** (BAA). In regulated industries — healthcare, finance,
government — that means prompts and code snippets sent through those
extensions can fall outside the contractual coverage your organization
relies on.

Microsoft Foundry, on the other hand, runs inside your Azure tenant under
the standard Microsoft Enterprise Agreement BAA. Foundry Copilot routes
every prompt, every completion, every agent step through Foundry — and only
Foundry — so the BAA boundary always holds.

## Why this matters

- **PHI / PII compliance:** the BAA is the contract that lets Microsoft handle
  Protected Health Information on your behalf. Sending PHI through a service
  that lacks a BAA is a contractual (and potentially regulatory) violation.
- **Data residency:** Foundry deployments live in the Azure region you choose.
  GitHub Copilot's request path is not configurable by tenant.
- **Audit:** all Foundry calls land in your Azure Monitor / Cost Management.
  Foundry Copilot's local telemetry (`telemetry.jsonl`) gives you a second
  audit trail that never leaves the machine unless you explicitly enable the
  OTLP exporter.

## What this extension does on activation

1. **Detects** GitHub Copilot extensions (`GitHub.copilot`,
   `GitHub.copilot-chat`, `GitHub.copilot-workspace`, `GitHub.copilot-labs`).
2. **Enforces** the configured policy. Default: silently disable them at
   workspace scope so your team's `.vscode/extensions.json` is the source of
   truth.
3. **Watches** for newly installed conflicting extensions and re-runs step 2.
4. **Surfaces** state in a left-anchored status-bar item: green-themed for
   enforced, yellow for unresolved conflicts, red for "BAA off".

## Configure

| Setting                                          | Default          | Effect                                                                                       |
| ------------------------------------------------ | ---------------- | -------------------------------------------------------------------------------------------- |
| `foundryCopilot.baa.enforcement`                 | `auto`           | `auto` disables silently. `prompt` asks once per conflict. `off` does nothing (UNPROTECTED). |
| `foundryCopilot.baa.conflictingExtensions`       | `[]` (extras)    | Additional extension IDs to treat as conflicts on top of the built-in list.                  |
| `foundryCopilot.baa.target`                      | `workspace`      | Scope to disable at. `workspace` is reversible; `global` persists across all your workspaces. |

## Status bar

The shield indicator on the left tells you the current state at all times:

- `$(shield) Foundry BAA` — enforced. No conflicts detected.
- `$(alert) Foundry BAA — N conflicts` — conflicting extensions are installed
  and the prompt mode is waiting on you. Click to disable them now.
- `$(error) Foundry BAA — OFF` — enforcement is disabled. Re-enable in
  Settings or click the indicator to invoke the disable-now command.

## When to choose `prompt` over `auto`

`prompt` is appropriate when a developer needs to keep both Foundry and
GitHub Copilot installed for non-PHI projects in separate workspaces. The
modal is shown the first time per workspace; subsequent activations are silent.

## When to choose `off`

Only when you have an out-of-band mechanism (managed extension policy via
Intune, for example) preventing the GitHub Copilot extensions from being
installed in the first place. Setting `off` makes the status bar **red** so
the situation is always visible.

## Want to dig deeper?

- See [SECURITY.md](command:vscode.open?%22vscode-remote%3A%2F%2Fmain%2FSECURITY.md%22)
  for the threat model and the two layered locks
  (`core/internal/foundry/lock.go`, `core/internal/control/lock.go`) that
  enforce the boundary at the transport layer.
- See the test matrix at [tests/MATRIX.md](command:vscode.open?%22vscode-remote%3A%2F%2Fmain%2Ftests%2FMATRIX.md%22)
  for the security regression suite (rows under "Security" and "Phase 11").
