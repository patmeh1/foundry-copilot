// status.ts wires the pure-TS guard to real VS Code surfaces:
//
// - A left-anchored status-bar item with three states (enforced / conflict /
//   off) that uses VS Code's themed status-bar background colours.
// - Three commands the user can drive directly:
//     foundryCopilot.baa.recheck
//     foundryCopilot.baa.disableConflicts
//     foundryCopilot.baa.showWalkthrough
// - A listener on vscode.extensions.onDidChange so installing or uninstalling
//   Copilot recomputes the state immediately.
import * as vscode from 'vscode';
import { BaaConfig, BaaState, DEFAULT_CONFLICTS, detectConflicts, enforce } from './guard';

const STATUS_PRIORITY = 100;
const SETTINGS_NAMESPACE = 'foundryCopilot.baa';

export class BaaStatusController implements vscode.Disposable {
    private readonly item: vscode.StatusBarItem;
    private readonly output: vscode.OutputChannel;
    private state: BaaState = { mode: 'off', conflicts: [] };
    private disposables: vscode.Disposable[] = [];

    constructor(output: vscode.OutputChannel) {
        this.output = output;
        this.item = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, STATUS_PRIORITY);
        this.item.command = 'foundryCopilot.baa.disableConflicts';
    }

    async start(context: vscode.ExtensionContext): Promise<void> {
        context.subscriptions.push(this);
        context.subscriptions.push(this.item);
        this.item.show();

        context.subscriptions.push(
            vscode.commands.registerCommand('foundryCopilot.baa.recheck', () => this.recheck()),
            vscode.commands.registerCommand('foundryCopilot.baa.disableConflicts', () => this.recheck(true)),
            vscode.commands.registerCommand('foundryCopilot.baa.showWalkthrough', () =>
                vscode.commands.executeCommand(
                    'workbench.action.openWalkthrough',
                    'patmeh1.foundry-copilot#foundryCopilot.baa',
                ),
            ),
        );

        this.disposables.push(vscode.extensions.onDidChange(() => this.recheck()));
        await this.recheck();
    }

    private readConfig(): BaaConfig {
        const c = vscode.workspace.getConfiguration(SETTINGS_NAMESPACE);
        const enforcement = c.get<'auto' | 'prompt' | 'off'>('enforcement', 'auto');
        const extras = c.get<string[]>('conflictingExtensions', []);
        const targetSetting = c.get<'workspace' | 'global'>('target', 'workspace');
        return {
            enforcement,
            conflictingExtensions: [...DEFAULT_CONFLICTS, ...extras],
            target:
                targetSetting === 'global'
                    ? vscode.ConfigurationTarget.Global
                    : vscode.ConfigurationTarget.Workspace,
        };
    }

    private async recheck(forceEnforce = false): Promise<void> {
        const cfg = this.readConfig();
        const conflicts = detectConflicts(vscode.extensions, cfg.conflictingExtensions);
        let mode: BaaState['mode'];
        if (cfg.enforcement === 'off' && !forceEnforce) {
            mode = 'off';
        } else if (conflicts.length === 0) {
            mode = 'enforced';
        } else {
            mode = 'conflict';
        }
        this.state = { mode, conflicts };
        if (conflicts.length > 0 && (cfg.enforcement !== 'off' || forceEnforce)) {
            this.state = await enforce(this.state, cfg, vscode.commands, vscode.window, (m) =>
                this.output.appendLine(m),
            );
        }
        this.render();
    }

    private render(): void {
        switch (this.state.mode) {
            case 'enforced':
                this.item.text = '$(shield) Foundry BAA';
                this.item.tooltip =
                    'Foundry Copilot is enforcing the BAA boundary. GitHub Copilot extensions are disabled in this workspace.';
                this.item.backgroundColor = undefined; // default themed background
                break;
            case 'conflict':
                this.item.text = `$(alert) Foundry BAA — ${this.state.conflicts.length} conflict${this.state.conflicts.length === 1 ? '' : 's'}`;
                this.item.tooltip = `BAA boundary at risk: the following extensions are installed and could route prompts outside the BAA:\n\n${this.state.conflicts.join('\n')}\n\nClick to disable them now.`;
                this.item.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
                break;
            case 'off':
                this.item.text = '$(error) Foundry BAA — OFF';
                this.item.tooltip =
                    'BAA enforcement is disabled (foundryCopilot.baa.enforcement = "off"). Foundry Copilot will not disable GitHub Copilot extensions automatically.';
                this.item.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
                break;
        }
    }

    dispose(): void {
        for (const d of this.disposables) d.dispose();
        this.disposables = [];
    }
}
