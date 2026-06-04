// inline-chat.ts — Cmd+I-style inline chat surface. v0.2 scaffold inserts a
// comment marker on the line above the cursor with the user's prompt so they
// can see the surface is wired end-to-end; v0.2.1 hands the prompt to the
// sidecar and renders a real inline diff.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

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
            void rpc;
            const line = editor.selection.active.line;
            const indent = editor.document.lineAt(line).text.match(/^\s*/)?.[0] ?? '';
            const comment = `${indent}// [Foundry inline chat stub] ${prompt}\n`;
            const we = new vscode.WorkspaceEdit();
            we.insert(editor.document.uri, new vscode.Position(line, 0), comment);
            await vscode.workspace.applyEdit(we);
            output.appendLine(`[inline-chat] inserted scaffold marker at line ${line + 1}`);
        }),
    );
}
