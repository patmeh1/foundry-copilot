// Inline completion provider — wires VS Code's InlineCompletionItemProvider
// to the sidecar's complete/inline RPC. Includes simple debouncing so we
// don't hammer the model on every keystroke.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

interface CompleteInlineReply {
    text: string;
}

const DEBOUNCE_MS = 200;

export function registerInlineCompletionProvider(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): vscode.Disposable {
    let lastTimer: NodeJS.Timeout | undefined;

    const provider: vscode.InlineCompletionItemProvider = {
        async provideInlineCompletionItems(document, position, _ctx, token) {
            if (lastTimer) clearTimeout(lastTimer);
            const debounced = new Promise<void>((resolve) => {
                lastTimer = setTimeout(resolve, DEBOUNCE_MS);
            });
            await debounced;
            if (token.isCancellationRequested) return undefined;

            const prefix = document.getText(new vscode.Range(new vscode.Position(0, 0), position));
            const lastLine = document.lineCount - 1;
            const end = new vscode.Position(lastLine, document.lineAt(lastLine).range.end.character);
            const suffix = document.getText(new vscode.Range(position, end));

            try {
                const reply = await rpc.request<CompleteInlineReply>('complete/inline', {
                    prefix,
                    suffix,
                    language: document.languageId,
                });
                if (token.isCancellationRequested) return undefined;
                const text = reply.text || '';
                if (!text) return undefined;
                return [new vscode.InlineCompletionItem(text, new vscode.Range(position, position))];
            } catch (err: unknown) {
                const m = err instanceof Error ? err.message : String(err);
                output.appendLine(`[completion] error: ${m}`);
                return undefined;
            }
        },
    };

    const disposable = vscode.languages.registerInlineCompletionItemProvider(
        { pattern: '**' },
        provider,
    );
    context.subscriptions.push(disposable);
    return disposable;
}
