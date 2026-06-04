// tests/diagnose.ts — "diagnose failed test" surface. Pulls the most
// recent diagnostic at severity Error from the active file (or the entire
// workspace) and asks Foundry to explain it.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

const SYSTEM_PROMPT =
    'You are Foundry. The user will paste a failing test diagnostic plus relevant code. ' +
    'Diagnose the root cause and propose a fix. Reply in two sections: ## Diagnosis and ## Fix.';

export function registerTestsDiagnose(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const channel = vscode.window.createOutputChannel('Foundry Diagnose');
    context.subscriptions.push(channel);
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.tests.diagnoseFailure', async () => {
            // 1. Find the most recent error-severity diagnostic.
            let target: { uri: vscode.Uri; diag: vscode.Diagnostic } | undefined;
            for (const [uri, diags] of vscode.languages.getDiagnostics()) {
                for (const d of diags) {
                    if (d.severity !== vscode.DiagnosticSeverity.Error) continue;
                    if (!target) {
                        target = { uri, diag: d };
                    }
                }
            }
            if (!target) {
                vscode.window.showInformationMessage('Foundry: no error diagnostics found.');
                return;
            }
            const doc = await vscode.workspace.openTextDocument(target.uri);
            // Pull a 30-line window around the diagnostic.
            const startLine = Math.max(0, target.diag.range.start.line - 15);
            const endLine = Math.min(doc.lineCount - 1, target.diag.range.end.line + 15);
            const codeWindow = doc.getText(new vscode.Range(startLine, 0, endLine, Number.MAX_SAFE_INTEGER));
            const streamId = `diag-${Date.now()}`;
            channel.show(true);
            channel.appendLine(`\n── Diagnosing: ${vscode.workspace.asRelativePath(target.uri)}:${target.diag.range.start.line + 1} ──`);
            const sub = rpc.onNotification('chat/chunk', (params) => {
                const c = params as { stream_id?: string; delta?: string; error?: string; finish_reason?: string };
                if (c.stream_id !== streamId) return;
                if (c.delta) channel.append(c.delta);
                if (c.error) channel.appendLine(`\n[error] ${c.error}`);
                if (c.finish_reason) channel.appendLine(`\n[done: ${c.finish_reason}]`);
            });
            const messages: ChatMessage[] = [
                { role: 'system', content: SYSTEM_PROMPT },
                {
                    role: 'user',
                    content:
                        `File: ${target.uri.fsPath}\nLanguage: ${doc.languageId}\n` +
                        `Diagnostic (${target.diag.source ?? 'unknown'}): ${target.diag.message}\n\n` +
                        `Code window (lines ${startLine + 1}-${endLine + 1}):\n${codeWindow}`,
                },
            ];
            try {
                await Methods.chatStart(rpc, { stream_id: streamId, messages, max_tokens: 1500 });
            } catch (err) {
                channel.appendLine(`\n[error] ${(err as Error).message}`);
                output.appendLine(`[tests] diagnose failed: ${(err as Error).message}`);
            } finally {
                sub.dispose();
            }
        }),
    );
}
