// nes/provider.ts — Next Edit Suggestions inline-completion provider.
// Gated on foundryCopilot.nes.enabled (default false) because the predictor
// requires extra latency and a workspace warmup step. v0.2 scaffold returns
// no completions; v0.2.1 wires nes/predict against the sidecar.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerNesProvider(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const enabled = vscode.workspace.getConfiguration('foundryCopilot.nes').get<boolean>('enabled', false);
    if (!enabled) {
        output.appendLine('[nes] disabled by foundryCopilot.nes.enabled');
        return;
    }
    const provider: vscode.InlineCompletionItemProvider = {
        async provideInlineCompletionItems(_doc, _pos, _ctx, _tok) {
            // v0.2 scaffold: no predictions yet.
            void rpc;
            return { items: [] };
        },
    };
    context.subscriptions.push(
        vscode.languages.registerInlineCompletionItemProvider({ pattern: '**' }, provider),
    );
    output.appendLine('[nes] provider registered (scaffold)');
}
