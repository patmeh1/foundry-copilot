// notebook-chat.ts — ask/fix/tests commands on the active notebook cell.
// Sends the cell content through chat/start with role-appropriate system
// prompts and renders the streamed answer in an output-channel preview.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

interface Surface {
    id: string;
    label: string;
    system: string;
}

const SURFACES: Surface[] = [
    {
        id: 'foundryCopilot.notebook.askCell',
        label: 'Ask about cell',
        system: 'You are Foundry. The user will share a notebook cell. Explain what it does concisely.',
    },
    {
        id: 'foundryCopilot.notebook.fixCell',
        label: 'Fix cell',
        system: 'You are Foundry. The user will share a broken notebook cell. Identify the bug and output the fixed cell wrapped in a fenced code block.',
    },
    {
        id: 'foundryCopilot.notebook.testsCell',
        label: 'Generate tests for cell',
        system: 'You are Foundry. The user will share a notebook cell. Generate unit tests (pytest if Python, otherwise the framework idiomatic to the language) wrapped in a fenced code block.',
    },
];

export function registerNotebookChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const channel = vscode.window.createOutputChannel('Foundry Notebook Chat');
    context.subscriptions.push(channel);
    for (const surface of SURFACES) {
        context.subscriptions.push(
            vscode.commands.registerCommand(surface.id, async () => {
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
                const language = cell.document.languageId;
                const body = cell.document.getText();
                const streamId = `nb-${surface.id}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
                channel.show(true);
                channel.appendLine(`\n── ${surface.label}: cell ${cell.index + 1} (${language}) ──`);
                const sub = rpc.onNotification('chat/chunk', (params) => {
                    const c = params as { stream_id?: string; delta?: string; error?: string; finish_reason?: string };
                    if (c.stream_id !== streamId) return;
                    if (c.delta) channel.append(c.delta);
                    if (c.error) channel.appendLine(`\n[error] ${c.error}`);
                    if (c.finish_reason) channel.appendLine(`\n[done: ${c.finish_reason}]`);
                });
                const messages: ChatMessage[] = [
                    { role: 'system', content: surface.system },
                    { role: 'user', content: `Language: ${language}\n\n${body}` },
                ];
                try {
                    await Methods.chatStart(rpc, { stream_id: streamId, messages });
                } catch (err) {
                    channel.appendLine(`\n[error] ${(err as Error).message}`);
                    output.appendLine(`[notebook-chat] ${surface.id} failed: ${(err as Error).message}`);
                } finally {
                    sub.dispose();
                }
            }),
        );
    }
}
