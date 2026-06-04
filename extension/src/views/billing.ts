// billing.ts — view stub for cost & quota.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerBillingView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    void rpc;
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.billing.open', () =>
            vscode.window.showInformationMessage(
                '[Foundry billing] Cost & quota dashboard lands in v0.2.1.',
            ),
        ),
    );
}
