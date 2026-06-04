// quick-chat.ts — palette-style one-shot question. Runs against the Foundry
// chat deployment via chat/start, streams chunks into a fresh output-channel
// preview, and surfaces a "Copy answer" action on completion.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

export function registerQuickChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const answerChannel = vscode.window.createOutputChannel('Foundry Quick Chat');
    context.subscriptions.push(answerChannel);
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.quickChat', async () => {
            const prompt = await vscode.window.showInputBox({
                prompt: 'Ask Foundry (quick)',
                placeHolder: 'What does this codebase do?',
            });
            if (!prompt) return;
            const streamId = `quick-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
            let buffer = '';
            const sub = rpc.onNotification('chat/chunk', (params) => {
                const c = params as { stream_id?: string; delta?: string; finish_reason?: string; error?: string };
                if (c.stream_id !== streamId) return;
                if (c.delta) {
                    buffer += c.delta;
                    answerChannel.append(c.delta);
                }
                if (c.error) answerChannel.appendLine(`\n[error] ${c.error}`);
                if (c.finish_reason) answerChannel.appendLine(`\n[done: ${c.finish_reason}]`);
            });
            answerChannel.clear();
            answerChannel.show(true);
            answerChannel.appendLine(`> ${prompt}\n`);
            const messages: ChatMessage[] = [
                { role: 'system', content: 'You are Foundry, a helpful coding assistant. Answer concisely.' },
                { role: 'user', content: prompt },
            ];
            try {
                await vscode.window.withProgress(
                    { location: vscode.ProgressLocation.Window, title: 'Foundry asking…' },
                    () => Methods.chatStart(rpc, { stream_id: streamId, messages }),
                );
            } catch (err) {
                answerChannel.appendLine(`\n[error] ${(err as Error).message}`);
                output.appendLine(`[quick-chat] failed: ${(err as Error).message}`);
            } finally {
                sub.dispose();
            }
            if (buffer) {
                const choice = await vscode.window.showInformationMessage('Foundry replied', 'Copy answer');
                if (choice === 'Copy answer') {
                    await vscode.env.clipboard.writeText(buffer);
                }
            }
        }),
    );
}
