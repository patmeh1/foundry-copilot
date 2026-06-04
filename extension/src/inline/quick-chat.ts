// quick-chat.ts — palette-style one-shot question. v0.2 scaffold echoes
// the prompt; v0.2.1 routes through chat/start and renders the response.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerQuickChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.quickChat', async () => {
            const prompt = await vscode.window.showInputBox({
                prompt: 'Ask Foundry (quick)',
                placeHolder: 'What does this codebase do?',
            });
            if (!prompt) return;
            void rpc;
            vscode.window.showInformationMessage(`[Foundry quick chat scaffold] You asked: ${prompt}`);
            output.appendLine(`[quick-chat] received prompt (${prompt.length} chars)`);
        }),
    );
}
