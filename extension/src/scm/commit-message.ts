// commit-message.ts — wires a button into the SCM view that asks Foundry
// to draft a commit message from the staged diff. Uses git.diff(true) to
// read the index, sends it to chat/start with a Conventional-Commits
// system prompt, then sets repo.inputBox.value to the streamed reply.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

interface GitRepo {
    rootUri: vscode.Uri;
    inputBox: { value: string };
    state: { indexChanges: { uri: vscode.Uri }[] };
    diff(cached?: boolean): Promise<string>;
}
interface GitAPI {
    repositories: GitRepo[];
}
interface GitExtension {
    getAPI(version: 1): GitAPI;
}

const SYSTEM_PROMPT =
    'You are Foundry. Write a Conventional Commits style commit message ' +
    '(type(scope): subject + body) from the staged diff the user supplies. ' +
    'Subject ≤ 72 chars, imperative mood, no trailing period. Body wrapped ' +
    'at 100 cols, explain WHY not WHAT. Output ONLY the commit message.';

export function registerCommitMessage(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.scm.generateCommitMessage', async () => {
            const gitExt = vscode.extensions.getExtension<GitExtension>('vscode.git');
            if (!gitExt) {
                vscode.window.showWarningMessage('Built-in Git extension is unavailable.');
                return;
            }
            const git = (await gitExt.activate()).getAPI(1);
            const repo = git.repositories[0];
            if (!repo) {
                vscode.window.showWarningMessage('No Git repository found in this workspace.');
                return;
            }
            let diff = '';
            try {
                diff = await repo.diff(true);
            } catch (err) {
                output.appendLine(`[scm] diff(true) failed: ${(err as Error).message}`);
            }
            if (!diff.trim()) {
                vscode.window.showInformationMessage('Foundry: no staged changes to summarize.');
                return;
            }
            // Hard cap diff size so we never blow the model context.
            if (diff.length > 40_000) diff = diff.slice(0, 40_000) + '\n…(diff truncated)';
            const streamId = `commit-${Date.now()}`;
            let buffer = '';
            const sub = rpc.onNotification('chat/chunk', (params) => {
                const c = params as { stream_id?: string; delta?: string };
                if (c.stream_id !== streamId || !c.delta) return;
                buffer += c.delta;
            });
            const messages: ChatMessage[] = [
                { role: 'system', content: SYSTEM_PROMPT },
                { role: 'user', content: diff },
            ];
            try {
                await vscode.window.withProgress(
                    { location: vscode.ProgressLocation.SourceControl, title: 'Foundry drafting commit…' },
                    () => Methods.chatStart(rpc, { stream_id: streamId, messages, max_tokens: 600 }),
                );
            } catch (err) {
                vscode.window.showErrorMessage(`Foundry commit-message failed: ${(err as Error).message}`);
                output.appendLine(`[scm] chat/start failed: ${(err as Error).message}`);
                sub.dispose();
                return;
            }
            sub.dispose();
            const msg = buffer.trim();
            if (!msg) {
                vscode.window.showWarningMessage('Foundry returned no commit message.');
                return;
            }
            repo.inputBox.value = msg;
            output.appendLine(`[scm] commit message written for ${repo.rootUri.fsPath} (${msg.length} chars)`);
        }),
    );
}
