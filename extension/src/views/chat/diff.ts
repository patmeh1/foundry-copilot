// diff.ts — propose / apply / discard for assistant-suggested file edits.
// v0.2.0 scaffold: minimal full-file replace semantics. v0.2.1 swaps to
// hunk-level diffs once the chat view returns structured edit blocks.
import * as vscode from 'vscode';

export interface ProposedFileEdit {
    path: string;
    before: string;
    after: string;
    rationale?: string;
}

export async function applyEdits(workspaceRoot: vscode.Uri, edits: ProposedFileEdit[]): Promise<void> {
    const we = new vscode.WorkspaceEdit();
    for (const e of edits) {
        const uri = vscode.Uri.joinPath(workspaceRoot, e.path);
        const doc = await vscode.workspace.openTextDocument(uri);
        const full = new vscode.Range(doc.positionAt(0), doc.positionAt(doc.getText().length));
        we.replace(uri, full, e.after);
    }
    await vscode.workspace.applyEdit(we);
}

export function discardEdits(edits: ProposedFileEdit[]): void {
    // No-op in v0.2.0 — UI just removes them from the proposal list.
    void edits;
}

export function renderDiff(edit: ProposedFileEdit): string {
    const beforeLines = edit.before.split('\n');
    const afterLines = edit.after.split('\n');
    const out: string[] = ['<pre>'];
    const max = Math.max(beforeLines.length, afterLines.length);
    for (let i = 0; i < max; i++) {
        const b = beforeLines[i];
        const a = afterLines[i];
        if (b === a) {
            out.push(`  ${escapeHtml(b ?? '')}`);
        } else {
            if (b !== undefined) out.push(`- ${escapeHtml(b)}`);
            if (a !== undefined) out.push(`+ ${escapeHtml(a)}`);
        }
    }
    out.push('</pre>');
    return out.join('\n');
}

function escapeHtml(s: string): string {
    return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]!));
}
