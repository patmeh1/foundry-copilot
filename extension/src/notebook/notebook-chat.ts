// notebook-chat.ts — ask/fix/tests commands on the active notebook cell.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerNotebookChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const surfaces = [
        { id: 'foundryCopilot.notebook.askCell', label: 'Ask about cell' },
        { id: 'foundryCopilot.notebook.fixCell', label: 'Fix cell' },
        { id: 'foundryCopilot.notebook.testsCell', label: 'Generate tests for cell' },
    ];
    for (const { id, label } of surfaces) {
        context.subscriptions.push(
            vscode.commands.registerCommand(id, async () => {
                const ed = vscode.window.activeNotebookEditor;
                if (!ed) {
                    vscode.window.showWarningMessage('Open a notebook first.');
                    return;
                }
                const sel = ed.selections[0];
                if (!sel) {
                    vscode.window.showWarningMessage('Select a cell first.');
                    return;
                }
                const cell = ed.notebook.cellAt(sel.start);
                void rpc;
                vscode.window.showInformationMessage(
                    `[Foundry notebook scaffold] ${label} — cell ${cell.index + 1} (${cell.document.lineCount} lines)`,
                );
                output.appendLine(`[notebook-chat] ${id} invoked on cell ${cell.index}`);
            }),
        );
    }
}
