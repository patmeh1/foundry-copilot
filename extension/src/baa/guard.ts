// guard.ts is the pure-TS BAA enforcement engine.
//
// "BAA" = Business Associate Agreement — the HIPAA contract that lets a
// cloud service handle Protected Health Information on a customer's behalf.
// GitHub Copilot and GitHub Copilot Chat do NOT ship with a BAA today.
// Microsoft Foundry does. This module exists so foundry-copilot can detect
// the GitHub Copilot extensions and disable them at activation time,
// preventing accidental PHI leakage through a non-BAA-covered service.
//
// The module is split this way:
//   - guard.ts (this file): pure TypeScript, no live vscode imports —
//     unit-testable with mocks.
//   - status.ts: the VS Code-facing wrapper (status bar + commands).
//
// The matrix tests in tests/MATRIX.md (rows 11.01–11.06) are pinned to
// this file's exports.

// Type-only import — keeps the module fully runnable under `node --test`
// without a vscode runtime.
import type * as vscode from 'vscode';

/** Default conflicting extension IDs. Reviewable in one place. */
export const DEFAULT_CONFLICTS = [
    'GitHub.copilot',
    'GitHub.copilot-chat',
    'GitHub.copilot-workspace',
    'GitHub.copilot-labs',
];

export type BaaEnforcement = 'auto' | 'prompt' | 'off';

export interface BaaConfig {
    enforcement: BaaEnforcement;
    conflictingExtensions: string[];
    target: vscode.ConfigurationTarget;
}

export interface BaaState {
    /** 'enforced' = guard ran successfully; 'conflict' = present but not yet disabled; 'off' = guard disabled. */
    mode: 'enforced' | 'conflict' | 'off';
    /** Currently-installed conflicting extension IDs. */
    conflicts: string[];
}

/** Test seam for `vscode.extensions`. */
export interface ExtensionsHost {
    getExtension(id: string): { id: string; isActive: boolean } | undefined;
}

/** Test seam for `vscode.commands`. */
export interface CommandHost {
    executeCommand(cmd: string, ...args: unknown[]): Thenable<unknown> | Promise<unknown>;
}

/** Test seam for the UI surface (warnings + info messages). */
export interface UiHost {
    showWarningMessage(
        msg: string,
        options: { modal?: boolean },
        ...items: string[]
    ): Thenable<string | undefined> | Promise<string | undefined>;
    showInformationMessage(msg: string): Thenable<string | undefined> | Promise<string | undefined>;
}

/** Returns the IDs from `conflictIds` whose extension is currently installed. */
export function detectConflicts(host: ExtensionsHost, conflictIds: string[]): string[] {
    const out: string[] = [];
    for (const id of conflictIds) {
        if (host.getExtension(id)) {
            out.push(id);
        }
    }
    return out;
}

/**
 * Apply the configured enforcement mode to the detected conflicts.
 *
 * - `'auto'`: invoke `workbench.extensions.disableExtension` for each conflict.
 *   No user prompt — the assumption is that customers in regulated industries
 *   *want* this to happen silently on activation.
 * - `'prompt'`: show a modal warning the first time conflicts are seen; the
 *   user picks "Disable all (workspace)" / "Disable all (globally)" / "Cancel".
 * - `'off'`: do nothing. The status bar will be RED to make the choice visible.
 *
 * Returns the post-enforcement state so the caller can refresh the status bar.
 */
export async function enforce(
    state: BaaState,
    cfg: BaaConfig,
    commands: CommandHost,
    ui: UiHost,
    log: (msg: string) => void,
): Promise<BaaState> {
    if (state.conflicts.length === 0) {
        log('[baa] no conflicts detected');
        return { mode: cfg.enforcement === 'off' ? 'off' : 'enforced', conflicts: [] };
    }
    if (cfg.enforcement === 'off') {
        log(`[baa] enforcement=off; ${state.conflicts.length} conflicts present (UNPROTECTED)`);
        return { mode: 'off', conflicts: state.conflicts };
    }
    if (cfg.enforcement === 'auto') {
        for (const id of state.conflicts) {
            log(`[baa] auto-disabling ${id}`);
            await commands.executeCommand('workbench.extensions.disableExtension', id);
        }
        return { mode: 'enforced', conflicts: [] };
    }
    // 'prompt'
    const choice = await ui.showWarningMessage(
        `Foundry Copilot detected ${state.conflicts.length} conflicting GitHub Copilot extension(s) without a BAA. Choose how to proceed:`,
        { modal: true },
        'Disable all (workspace)',
        'Disable all (globally)',
        'Cancel',
    );
    if (choice === 'Disable all (workspace)' || choice === 'Disable all (globally)') {
        for (const id of state.conflicts) {
            log(`[baa] prompt-disabling ${id} (${choice})`);
            await commands.executeCommand('workbench.extensions.disableExtension', id);
        }
        return { mode: 'enforced', conflicts: [] };
    }
    log(`[baa] user dismissed prompt; ${state.conflicts.length} conflicts still present`);
    return { mode: 'conflict', conflicts: state.conflicts };
}
