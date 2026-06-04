// commit-message.ts — wire a button into the SCM view that asks Foundry for
// a commit message. v0.2 scaffold sets a deterministic placeholder.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

interface GitRepo {
    rootUri: vscode.Uri;
    inputBox: { value: string };
    state: { indexChanges: { uri: vscode.Uri }[] };
}
interface GitAPI {
    repositories: GitRepo[];
}
interface GitExtension {
    getAPI(version: 1): GitAPI;
}

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
            void rpc;
            const first = repo.state.indexChanges[0];
            const hint = first ? vscode.workspace.asRelativePath(first.uri) : 'change';
            repo.inputBox.value = `chore: ${hint} [foundry scaffold]`;
            output.appendLine(`[scm] commit message scaffold written for ${repo.rootUri.fsPath}`);
        }),
    );
}
