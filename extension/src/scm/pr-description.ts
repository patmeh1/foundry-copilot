// pr-description.ts — drafts a PR description from `git log origin/<base>..HEAD`
// plus the cumulative diff and copies it to the clipboard. The GitHub PR
// extension is not required (we only use it when available to pre-fill the
// PR description input box); the diff comes straight from the built-in Git
// extension.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

interface GitRepo {
    rootUri: vscode.Uri;
    state: { HEAD?: { upstream?: { name: string; remote: string } } };
    diffWith(ref: string): Promise<string>;
    log(opts?: { maxEntries?: number }): Promise<{ message: string }[]>;
}
interface GitAPI { repositories: GitRepo[]; }
interface GitExtension { getAPI(version: 1): GitAPI; }

const SYSTEM_PROMPT =
    'You are Foundry. Draft a Markdown pull request description from the diff and commit log the user supplies. ' +
    'Sections: ## Summary, ## Changes (bulleted), ## Risk / Rollback, ## Test plan. Be specific. Output ONLY the markdown.';

export function registerPrDescription(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.scm.generatePrDescription', async () => {
            const gitExt = vscode.extensions.getExtension<GitExtension>('vscode.git');
            if (!gitExt) {
                vscode.window.showWarningMessage('Built-in Git extension is unavailable.');
                return;
            }
            const git = (await gitExt.activate()).getAPI(1);
            const repo = git.repositories[0];
            if (!repo) {
                vscode.window.showWarningMessage('No Git repository found.');
                return;
            }
            const upstream = repo.state.HEAD?.upstream;
            const base = upstream ? `${upstream.remote}/${upstream.name}` : 'origin/main';
            let diff = '';
            let log: { message: string }[] = [];
            try {
                diff = await repo.diffWith(base);
                log = await repo.log({ maxEntries: 20 });
            } catch (err) {
                output.appendLine(`[scm] diff/log failed against ${base}: ${(err as Error).message}`);
            }
            if (!diff.trim()) {
                vscode.window.showInformationMessage(`Foundry: no diff against ${base}.`);
                return;
            }
            if (diff.length > 60_000) diff = diff.slice(0, 60_000) + '\n…(diff truncated)';
            const commits = log.map((l, i) => `${i + 1}. ${l.message.split('\n')[0]}`).join('\n');
            const streamId = `pr-${Date.now()}`;
            let buffer = '';
            const sub = rpc.onNotification('chat/chunk', (params) => {
                const c = params as { stream_id?: string; delta?: string };
                if (c.stream_id !== streamId || !c.delta) return;
                buffer += c.delta;
            });
            const messages: ChatMessage[] = [
                { role: 'system', content: SYSTEM_PROMPT },
                {
                    role: 'user',
                    content:
                        `Base: ${base}\n\nRecent commits:\n${commits}\n\n` +
                        `Cumulative diff:\n${diff}`,
                },
            ];
            try {
                await vscode.window.withProgress(
                    { location: vscode.ProgressLocation.SourceControl, title: 'Foundry drafting PR…' },
                    () => Methods.chatStart(rpc, { stream_id: streamId, messages, max_tokens: 1500 }),
                );
            } catch (err) {
                sub.dispose();
                vscode.window.showErrorMessage(`Foundry PR draft failed: ${(err as Error).message}`);
                return;
            }
            sub.dispose();
            const md = buffer.trim();
            if (!md) {
                vscode.window.showWarningMessage('Foundry returned an empty PR description.');
                return;
            }
            await vscode.env.clipboard.writeText(md);
            const choice = await vscode.window.showInformationMessage(
                `Foundry drafted a PR description (${md.length} chars). Copied to clipboard.`,
                'Preview',
            );
            if (choice === 'Preview') {
                const doc = await vscode.workspace.openTextDocument({ content: md, language: 'markdown' });
                await vscode.window.showTextDocument(doc, { preview: true });
            }
            output.appendLine(`[scm] PR description drafted (${md.length} chars)`);
        }),
    );
}
