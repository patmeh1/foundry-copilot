// guard.test.ts — covers test matrix rows 11.01-11.06.
//
// Uses Node's built-in test runner so we don't add jest/mocha as deps.
// Run with: node --test out/baa/guard.test.js (after `npm run build`).
import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import {
    BaaConfig,
    BaaState,
    CommandHost,
    DEFAULT_CONFLICTS,
    ExtensionsHost,
    UiHost,
    detectConflicts,
    enforce,
} from './guard';

// vscode.ConfigurationTarget enum values: Global=1, Workspace=2, WorkspaceFolder=3.
// We don't import vscode at runtime in unit tests, so use the numeric form.
const WORKSPACE = 2 as unknown as import('vscode').ConfigurationTarget;

function mkExt(installed: string[]): ExtensionsHost {
    return {
        getExtension: (id) => (installed.includes(id) ? { id, isActive: true } : undefined),
    };
}

class FakeCommands implements CommandHost {
    calls: { cmd: string; args: unknown[] }[] = [];
    async executeCommand(cmd: string, ...args: unknown[]) {
        this.calls.push({ cmd, args });
        return undefined;
    }
}

class FakeUi implements UiHost {
    warningCalls: { msg: string; modal: boolean; items: string[] }[] = [];
    nextChoice: string | undefined = undefined;
    async showWarningMessage(msg: string, options: { modal?: boolean }, ...items: string[]) {
        this.warningCalls.push({ msg, modal: !!options.modal, items });
        return this.nextChoice;
    }
    async showInformationMessage(_msg: string) {
        return undefined;
    }
}

const logs: string[] = [];
const log = (m: string) => logs.push(m);

test('11.01 detectConflicts returns GitHub.copilot when only that extension installed', () => {
    const ids = detectConflicts(mkExt(['GitHub.copilot']), DEFAULT_CONFLICTS);
    assert.deepEqual(ids, ['GitHub.copilot']);
});

test('11.02 detectConflicts returns GitHub.copilot-chat when only that extension installed', () => {
    const ids = detectConflicts(mkExt(['GitHub.copilot-chat']), DEFAULT_CONFLICTS);
    assert.deepEqual(ids, ['GitHub.copilot-chat']);
});

test('11.03 detectConflicts returns [] when no conflicting extension installed', () => {
    const ids = detectConflicts(mkExt(['some.other-ext']), DEFAULT_CONFLICTS);
    assert.deepEqual(ids, []);
});

test('11.04 enforce("auto") invokes disable per conflict', async () => {
    const commands = new FakeCommands();
    const ui = new FakeUi();
    const cfg: BaaConfig = { enforcement: 'auto', conflictingExtensions: DEFAULT_CONFLICTS, target: WORKSPACE };
    const state: BaaState = { mode: 'conflict', conflicts: ['GitHub.copilot', 'GitHub.copilot-chat'] };
    const result = await enforce(state, cfg, commands, ui, log);
    assert.equal(result.mode, 'enforced');
    assert.equal(commands.calls.length, 2);
    assert.equal(commands.calls[0].cmd, 'workbench.extensions.disableExtension');
    assert.equal(commands.calls[0].args[0], 'GitHub.copilot');
    assert.equal(commands.calls[1].args[0], 'GitHub.copilot-chat');
});

test('11.05 enforce("prompt") shows modal warning once', async () => {
    const commands = new FakeCommands();
    const ui = new FakeUi();
    ui.nextChoice = 'Disable all (workspace)';
    const cfg: BaaConfig = { enforcement: 'prompt', conflictingExtensions: DEFAULT_CONFLICTS, target: WORKSPACE };
    const state: BaaState = { mode: 'conflict', conflicts: ['GitHub.copilot'] };
    const result = await enforce(state, cfg, commands, ui, log);
    assert.equal(result.mode, 'enforced');
    assert.equal(ui.warningCalls.length, 1);
    assert.equal(ui.warningCalls[0].modal, true);
    assert.equal(commands.calls.length, 1);
});

test('11.05b enforce("prompt") respects Cancel', async () => {
    const commands = new FakeCommands();
    const ui = new FakeUi();
    ui.nextChoice = 'Cancel';
    const cfg: BaaConfig = { enforcement: 'prompt', conflictingExtensions: DEFAULT_CONFLICTS, target: WORKSPACE };
    const state: BaaState = { mode: 'conflict', conflicts: ['GitHub.copilot'] };
    const result = await enforce(state, cfg, commands, ui, log);
    assert.equal(result.mode, 'conflict');
    assert.equal(commands.calls.length, 0);
});

test('11.06 enforce("off") does not call executeCommand', async () => {
    const commands = new FakeCommands();
    const ui = new FakeUi();
    const cfg: BaaConfig = { enforcement: 'off', conflictingExtensions: DEFAULT_CONFLICTS, target: WORKSPACE };
    const state: BaaState = { mode: 'conflict', conflicts: ['GitHub.copilot'] };
    const result = await enforce(state, cfg, commands, ui, log);
    assert.equal(result.mode, 'off');
    assert.equal(commands.calls.length, 0);
});
