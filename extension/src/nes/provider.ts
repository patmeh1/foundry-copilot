// nes/provider.ts — Next Edit Suggestions inline-completion provider.
// Gated on foundryCopilot.nes.enabled (default false). The provider calls
// nes/predict on the sidecar with the cursor context; if the sidecar
// returns a non-empty suggestion text it is rendered as an inline ghost
// completion at the current position.
import * as vscode from 'vscode';
import { Methods, RpcClient } from '../sidecar/rpc';

const MAX_CONTEXT = 4096;

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
        async provideInlineCompletionItems(doc, pos, _ctx, tok) {
            try {
                const offset = doc.offsetAt(pos);
                const text = doc.getText();
                const before = text.slice(Math.max(0, offset - MAX_CONTEXT), offset);
                const after = text.slice(offset, Math.min(text.length, offset + MAX_CONTEXT));
                if (tok.isCancellationRequested) return { items: [] };
                const reply = await Methods.nesPredict(rpc, {
                    file: doc.uri.fsPath,
                    line: pos.line,
                    column: pos.character,
                    before, after,
                });
                if (tok.isCancellationRequested) return { items: [] };
                const sug = reply.suggestion;
                if (!sug?.text) return { items: [] };
                return {
                    items: [
                        new vscode.InlineCompletionItem(
                            sug.text,
                            new vscode.Range(pos, pos),
                        ),
                    ],
                };
            } catch (err) {
                output.appendLine(`[nes] predict failed: ${(err as Error).message}`);
                return { items: [] };
            }
        },
    };
    context.subscriptions.push(
        vscode.languages.registerInlineCompletionItemProvider({ pattern: '**' }, provider),
    );
    output.appendLine('[nes] provider registered');
}
