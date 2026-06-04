// pr-description.ts — placeholder for the PR description generator. Will
// invoke the GitHub PR extension's API once Foundry-backed generation is
// wired in v0.2.1.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerPrDescription(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.scm.generatePrDescription', async () => {
            const prExt = vscode.extensions.getExtension('GitHub.vscode-pull-request-github');
            if (!prExt) {
                vscode.window.showWarningMessage(
                    'Install the GitHub Pull Requests extension to use this feature.',
                );
                return;
            }
            void rpc;
            vscode.window.showInformationMessage(
                '[Foundry PR scaffold] PR description generation lands in v0.2.1.',
            );
            output.appendLine('[scm] pr description scaffold invoked');
        }),
    );
}
