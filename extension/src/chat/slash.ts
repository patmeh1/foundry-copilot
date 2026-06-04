// Slash-command helpers used by the @foundry chat participant.
// Each function builds a tailored {system, user} pair from the active
// editor's selection or whole file. The participant routes each /slash
// to the corresponding builder and then streams via chat/start.
import * as vscode from 'vscode';
import { ChatMessage } from '../sidecar/rpc';

export interface EditorContext {
    fileName: string;
    languageId: string;
    selection: string;
    selectionRange: { startLine: number; endLine: number } | undefined;
    fullText: string;
}

const MAX_CONTEXT_CHARS = 16 * 1024;

export function captureEditorContext(): EditorContext | undefined {
    const ed = vscode.window.activeTextEditor;
    if (!ed) return undefined;
    const doc = ed.document;
    const sel = ed.selection;
    const hasSelection = !sel.isEmpty;
    const selectionText = hasSelection ? doc.getText(sel) : '';
    const fullText = doc.getText();
    return {
        fileName: vscode.workspace.asRelativePath(doc.uri),
        languageId: doc.languageId,
        selection: clip(selectionText, MAX_CONTEXT_CHARS),
        selectionRange: hasSelection
            ? { startLine: sel.start.line + 1, endLine: sel.end.line + 1 }
            : undefined,
        fullText: clip(fullText, MAX_CONTEXT_CHARS),
    };
}

function clip(s: string, n: number): string {
    if (s.length <= n) return s;
    return s.slice(0, n) + '\n…[truncated]…';
}

export function buildExplain(
    request: vscode.ChatRequest,
    ctx: EditorContext | undefined,
): ChatMessage[] {
    const system =
        'You are Foundry Copilot. Explain the user-provided code clearly and ' +
        'concisely. Focus on what it does, why, edge cases, and any subtle ' +
        'behaviour. Use short bullets and a one-paragraph summary at the top.';
    return withContext(system, request, ctx, 'Explain this code.');
}

export function buildFix(
    request: vscode.ChatRequest,
    ctx: EditorContext | undefined,
): ChatMessage[] {
    const system =
        'You are Foundry Copilot. Identify the bug(s) in the supplied code and ' +
        'propose a minimal fix. Output a brief diagnosis (≤3 bullets) followed ' +
        'by a single code block containing the corrected snippet. Preserve the ' +
        'surrounding style and indentation.';
    return withContext(system, request, ctx, 'Find and fix bugs in this code.');
}

export function buildTests(
    request: vscode.ChatRequest,
    ctx: EditorContext | undefined,
): ChatMessage[] {
    const system =
        'You are Foundry Copilot. Generate unit tests for the supplied code. ' +
        'Use the language\'s idiomatic testing framework. Cover happy path, ' +
        'edge cases, and at least one failure mode. Output one code block.';
    return withContext(system, request, ctx, 'Write tests for this code.');
}

export function buildDoc(
    request: vscode.ChatRequest,
    ctx: EditorContext | undefined,
): ChatMessage[] {
    const system =
        'You are Foundry Copilot. Write idiomatic documentation comments ' +
        '(docstrings / doc comments) for the supplied code in the conventional ' +
        'style for the language. Do not change the code; only return the ' +
        'documented version in a single code block.';
    return withContext(system, request, ctx, 'Document this code.');
}

function withContext(
    system: string,
    request: vscode.ChatRequest,
    ctx: EditorContext | undefined,
    defaultPrompt: string,
): ChatMessage[] {
    const prompt = request.prompt.trim() || defaultPrompt;
    const msgs: ChatMessage[] = [{ role: 'system', content: system }];
    if (ctx) {
        const code = ctx.selection || ctx.fullText;
        const scope = ctx.selectionRange
            ? `selection lines ${ctx.selectionRange.startLine}–${ctx.selectionRange.endLine}`
            : 'whole file';
        msgs.push({
            role: 'user',
            content: [
                `FILE: ${ctx.fileName} (${ctx.languageId}, ${scope})`,
                '',
                '```' + ctx.languageId,
                code,
                '```',
                '',
                prompt,
            ].join('\n'),
        });
    } else {
        msgs.push({
            role: 'user',
            content: prompt + '\n\n(no active editor — please paste the code you want me to look at)',
        });
    }
    return msgs;
}
