// terminal-chat.ts — synthesises a shell command from a natural-language
// prompt using chat/start, then prompts the user to confirm before
// dispatching it to the active terminal. Refuses anything that looks like
// `rm -rf`, `sudo`, or output redirection without an explicit re-confirm.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

const DANGEROUS_PATTERNS = [
    /\brm\s+-rf?\b/,
    /\bsudo\b/,
    /\b(curl|wget)\s+[^|]*\|\s*(sh|bash)\b/,
    /\bdd\s+if=/,
    /\bmkfs\b/,
    /\b:>\s*\//, // truncate root file
];

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
            const streamId = `term-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
            let buffer = '';
            const sub = rpc.onNotification('chat/chunk', (params) => {
                const c = params as { stream_id?: string; delta?: string };
                if (c.stream_id !== streamId || !c.delta) return;
                buffer += c.delta;
            });
            const messages: ChatMessage[] = [
                {
                    role: 'system',
                    content:
                        'You convert natural-language requests into a single safe POSIX shell command. ' +
                        'Output ONLY the command (no fences, no explanation, no surrounding quotes). ' +
                        'Refuse anything destructive by emitting a single `# refused` line.',
                },
                { role: 'user', content: prompt },
            ];
            try {
                await vscode.window.withProgress(
                    { location: vscode.ProgressLocation.Window, title: 'Foundry composing command…' },
                    () => Methods.chatStart(rpc, { stream_id: streamId, messages, max_tokens: 200 }),
                );
            } catch (err) {
                sub.dispose();
                vscode.window.showErrorMessage(`Foundry terminal chat failed: ${(err as Error).message}`);
                output.appendLine(`[terminal-chat] failed: ${(err as Error).message}`);
                return;
            }
            sub.dispose();
            const cmd = buffer.trim().split('\n')[0].trim();
            if (!cmd || cmd.startsWith('#')) {
                vscode.window.showWarningMessage(`Foundry refused or returned no command: ${buffer.trim() || '(empty)'}`);
                return;
            }
            const dangerous = DANGEROUS_PATTERNS.some((re) => re.test(cmd));
            const choice = await vscode.window.showWarningMessage(
                `${dangerous ? 'DANGEROUS COMMAND.\n\n' : 'Foundry proposes:\n\n'}${cmd}\n\nRun it?`,
                { modal: true },
                'Run', 'Copy', 'Cancel',
            );
            if (choice === 'Copy') {
                await vscode.env.clipboard.writeText(cmd);
                return;
            }
            if (choice !== 'Run') return;
            const term = vscode.window.activeTerminal ?? vscode.window.createTerminal('Foundry');
            term.show(true);
            term.sendText(cmd);
            output.appendLine(`[terminal-chat] dispatched: ${cmd}`);
        }),
    );
}
