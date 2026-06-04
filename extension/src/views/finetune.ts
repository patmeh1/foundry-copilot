// finetune.ts — view stub for fine-tuning job orchestration.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

export function registerFinetuneView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    void rpc;
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.finetune.open', () =>
            vscode.window.showInformationMessage(
                '[Foundry finetune] Job board lands in v0.2.1. Dataset builder is available via RPC today.',
            ),
        ),
    );
}
