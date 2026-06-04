// inline-chat.ts — Cmd+I-style inline chat surface. Takes the user's
// selection (or the full file when nothing is selected) and routes it
// through chat/edit_propose, then renders a Preview/Apply confirmation
// before mutating the buffer with a WorkspaceEdit.
import * as vscode from 'vscode';
import { Methods, RpcClient } from '../sidecar/rpc';

export function registerInlineChat(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.inlineChat', async () => {
            const editor = vscode.window.activeTextEditor;
            if (!editor) {
                vscode.window.showWarningMessage('Open a file first.');
                return;
            }
            const prompt = await vscode.window.showInputBox({
                prompt: 'Inline chat (Foundry)',
                placeHolder: 'Refactor this to use async/await…',
            });
            if (!prompt) return;
            const doc = editor.document;
            const sel = editor.selection;
            const fullRange = new vscode.Range(
                doc.positionAt(0),
                doc.positionAt(doc.getText().length),
            );
            const targetRange = sel.isEmpty ? fullRange : sel;
            const originalText = doc.getText(targetRange);
            try {
                const reply = await vscode.window.withProgress(
                    { location: vscode.ProgressLocation.Notification, title: 'Foundry editing…' },
                    () => Methods.chatEditPropose(rpc, {
                        path: doc.uri.fsPath,
                        language: doc.languageId,
                        original_text: originalText,
                        instruction: prompt,
                    }),
                );
                const choice = await vscode.window.showInformationMessage(
                    `Foundry proposes an edit (${reply.new_text.length} chars). ${reply.explanation || ''}`,
                    { modal: true },
                    'Apply', 'Show diff',
                );
                if (choice === 'Show diff') {
                    const tmp = await vscode.workspace.openTextDocument({ content: reply.new_text, language: doc.languageId });
                    await vscode.commands.executeCommand('vscode.diff', doc.uri, tmp.uri, `Foundry edit: ${doc.fileName}`);
                    return;
                }
                if (choice !== 'Apply') return;
                const we = new vscode.WorkspaceEdit();
                we.replace(doc.uri, targetRange, reply.new_text);
                const ok = await vscode.workspace.applyEdit(we);
                if (ok) output.appendLine(`[inline-chat] applied edit (${reply.new_text.length} chars)`);
                else vscode.window.showWarningMessage('Foundry inline edit was not applied.');
            } catch (err) {
                vscode.window.showErrorMessage(`Foundry inline chat failed: ${(err as Error).message}`);
                output.appendLine(`[inline-chat] failed: ${(err as Error).message}`);
            }
        }),
    );
}
