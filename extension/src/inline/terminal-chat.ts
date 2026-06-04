// terminal-chat.ts — confirms a proposed shell command before sending it.
// v0.2 scaffold uses a fixed echo command; v0.2.1 asks the sidecar to
// synthesize the command from a natural-language prompt.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerTerminalChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.terminalChat', async () => {
            const prompt = await vscode.window.showInputBox({
                prompt: 'Terminal chat (Foundry)',
                placeHolder: 'list all running docker containers',
            });
            if (!prompt) return;
            void rpc;
            const proposed = `echo "[Foundry terminal-chat scaffold] would run: ${prompt.replace(/"/g, '\\"')}"`;
            const choice = await vscode.window.showWarningMessage(
                `Foundry proposes:\n\n${proposed}\n\nRun it?`,
                { modal: true },
                'Run',
                'Cancel',
            );
            if (choice !== 'Run') return;
            const term =
                vscode.window.activeTerminal ?? vscode.window.createTerminal('Foundry');
            term.show(true);
            term.sendText(proposed);
            output.appendLine(`[terminal-chat] sent scaffold command`);
        }),
    );
}
