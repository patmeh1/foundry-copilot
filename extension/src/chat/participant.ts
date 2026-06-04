// Foundry chat participant — appears in the VS Code chat view as @foundry.
// Bridges VS Code's chat protocol to the sidecar's chat/start + chat/chunk
// JSON-RPC notification stream. Slash command /agent routes through the
// agent/run RPC instead (tool-using loop).
import * as vscode from 'vscode';
import {
    AgentEvent,
    ChatChunk,
    ChatMessage,
    Methods,
    RpcClient,
} from '../sidecar/rpc';
import {
    buildDoc,
    buildExplain,
    buildFix,
    buildTests,
    captureEditorContext,
} from './slash';

const PARTICIPANT_ID = 'foundryCopilot.chat';

export function registerChatParticipant(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): vscode.Disposable {
    const handler: vscode.ChatRequestHandler = async (
        request,
        chatContext,
        response,
        token,
    ) => {
        switch (request.command) {
            case 'agent':
                return runAgent(request, response, token, rpc, output);
            case 'explain':
                return runSlash(buildExplain(request, captureEditorContext()), response, token, rpc, output);
            case 'fix':
                return runSlash(buildFix(request, captureEditorContext()), response, token, rpc, output);
            case 'tests':
                return runSlash(buildTests(request, captureEditorContext()), response, token, rpc, output);
            case 'doc':
                return runSlash(buildDoc(request, captureEditorContext()), response, token, rpc, output);
        }
        return runChat(request, chatContext, response, token, rpc, output);
    };

    const participant = vscode.chat.createChatParticipant(PARTICIPANT_ID, handler);
    participant.iconPath = new vscode.ThemeIcon('sparkle');
    context.subscriptions.push(participant);
    return participant;
}

// ─── /chat (default) ──────────────────────────────────────────────────────

async function runChat(
    request: vscode.ChatRequest,
    chatContext: vscode.ChatContext,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): Promise<void> {
    const messages = buildMessages(request, chatContext);
    return streamChat(messages, response, token, rpc, output);
}

// ─── /explain, /fix, /tests, /doc share one streamer ──────────────────────

async function runSlash(
    messages: ChatMessage[],
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): Promise<void> {
    return streamChat(messages, response, token, rpc, output);
}

async function streamChat(
    messages: ChatMessage[],
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): Promise<void> {
    const streamId = `s-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    const sub = rpc.onNotification('chat/chunk', (params) => {
        const chunk = params as ChatChunk;
        if (chunk.stream_id !== streamId) return;
        if (chunk.error) {
            response.markdown(`\n\n> **Error:** ${chunk.error}`);
            return;
        }
        if (chunk.delta) {
            response.markdown(chunk.delta);
        }
    });
    const cancel = token.onCancellationRequested(() => {
        output.appendLine(`[chat] cancellation requested for ${streamId}`);
    });
    try {
        const reply = await Methods.chatStart(rpc, { stream_id: streamId, messages });
        output.appendLine(`[chat] stream ${streamId} done finish=${reply.finish_reason}`);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        response.markdown(`\n\n> **Sidecar error:** ${m}`);
    } finally {
        sub.dispose();
        cancel.dispose();
    }
}

// ─── /agent ────────────────────────────────────────────────────────────────

async function runAgent(
    request: vscode.ChatRequest,
    response: vscode.ChatResponseStream,
    token: vscode.CancellationToken,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): Promise<void> {
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!folder) {
        response.markdown('> **/agent requires an open workspace folder.**');
        return;
    }
    const streamId = `a-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    const sub = rpc.onNotification('agent/event', (params) => {
        const ev = params as AgentEvent;
        if (ev.stream_id !== streamId) return;
        renderEvent(response, ev);
    });
    const cancel = token.onCancellationRequested(() => {
        output.appendLine(`[agent] cancellation requested for ${streamId}`);
    });
    try {
        response.markdown(`*Running agent on \`${folder.uri.fsPath}\`…*\n\n`);
        const reply = await Methods.agentRun(rpc, {
            stream_id: streamId,
            task: request.prompt,
            workdir: folder.uri.fsPath,
        });
        output.appendLine(`[agent] stream ${streamId} done steps=${reply.steps_taken}`);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        response.markdown(`\n\n> **Agent error:** ${m}`);
    } finally {
        sub.dispose();
        cancel.dispose();
    }
}

function renderEvent(response: vscode.ChatResponseStream, ev: AgentEvent): void {
    switch (ev.kind) {
        case 'think':
            if (ev.text) response.markdown(`\n${ev.text}\n`);
            return;
        case 'tool_call':
            response.markdown(`\n\n**↳ ${ev.tool}**`);
            if (ev.args && ev.args !== '{}') {
                response.markdown(`\n\`\`\`json\n${truncate(ev.args, 400)}\n\`\`\``);
            }
            return;
        case 'tool_result':
            if (ev.tool_error) {
                response.markdown(`\n> **tool error (${ev.tool}):** ${ev.tool_error}`);
                return;
            }
            if (ev.result) {
                response.markdown(`\n\`\`\`\n${truncate(ev.result, 600)}\n\`\`\``);
            }
            return;
        case 'final':
            if (ev.text) response.markdown(`\n\n${ev.text}\n`);
            return;
        case 'error':
            response.markdown(`\n\n> **agent error:** ${ev.text ?? 'unknown'}`);
            return;
    }
}

function truncate(s: string, n: number): string {
    if (s.length <= n) return s;
    return s.slice(0, n) + '\n…[truncated]…';
}

// ─── history → messages ────────────────────────────────────────────────────

function buildMessages(
    request: vscode.ChatRequest,
    chatContext: vscode.ChatContext,
): ChatMessage[] {
    const msgs: ChatMessage[] = [{
        role: 'system',
        content: 'You are Foundry Copilot — a coding assistant running on Microsoft Foundry. Be concise and accurate.',
    }];
    for (const turn of chatContext.history) {
        if (turn instanceof vscode.ChatRequestTurn) {
            msgs.push({ role: 'user', content: turn.prompt });
        } else if (turn instanceof vscode.ChatResponseTurn) {
            const text = collectResponseText(turn);
            if (text) msgs.push({ role: 'assistant', content: text });
        }
    }
    msgs.push({ role: 'user', content: request.prompt });
    return msgs;
}

function collectResponseText(turn: vscode.ChatResponseTurn): string {
    let out = '';
    for (const part of turn.response) {
        if (part instanceof vscode.ChatResponseMarkdownPart) {
            out += part.value.value;
        }
    }
    return out;
}

