// tests/diagnose.ts — "diagnose failed test" surface. v0.2 scaffold opens
// an info message; v0.2.1 streams a real diagnosis from the sidecar.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerTestsDiagnose(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.tests.diagnoseFailure', async () => {
            void rpc;
            vscode.window.showInformationMessage(
                '[Foundry tests scaffold] Diagnose-failure flow lands in v0.2.1.',
            );
            output.appendLine('[tests] diagnose scaffold invoked');
        }),
    );
}
