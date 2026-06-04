// telemetry.ts — tree view stub for telemetry summary.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerTelemetryView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    void rpc;
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.telemetry.open', () =>
            vscode.window.showInformationMessage(
                '[Foundry telemetry] Summary view lands in v0.2.1. Until then run `telemetry/summary` directly against the sidecar.',
            ),
        ),
    );
}
