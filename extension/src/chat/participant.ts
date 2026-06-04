// Foundry chat participant — appears in the VS Code chat view as @foundry.
// Bridges VS Code's chat protocol to the sidecar's chat/start + chat/chunk
// JSON-RPC notification stream.
import * as vscode from 'vscode';
import { ChatChunk, ChatMessage, Methods, RpcClient } from '../sidecar/rpc';

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
        const messages = buildMessages(request, chatContext);
        const streamId = `s-${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

        // Subscribe BEFORE making the request so we don't miss early chunks.
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

        // Wire cancellation: VS Code lets the user stop a request.
        const cancel = token.onCancellationRequested(() => {
            output.appendLine(`[chat] cancellation requested for ${streamId}`);
            // We don't have a cancel RPC yet — Phase 5 will add chat/cancel.
            // For now the sidecar will finish; the response just won't render.
        });

        try {
            const reply = await Methods.chatStart(rpc, {
                stream_id: streamId,
                messages,
            });
            output.appendLine(`[chat] stream ${streamId} done finish=${reply.finish_reason}`);
        } catch (err: unknown) {
            const m = err instanceof Error ? err.message : String(err);
            response.markdown(`\n\n> **Sidecar error:** ${m}`);
        } finally {
            sub.dispose();
            cancel.dispose();
        }
    };

    const participant = vscode.chat.createChatParticipant(PARTICIPANT_ID, handler);
    participant.iconPath = new vscode.ThemeIcon('sparkle');
    context.subscriptions.push(participant);
    return participant;
}

function buildMessages(
    request: vscode.ChatRequest,
    chatContext: vscode.ChatContext,
): ChatMessage[] {
    const msgs: ChatMessage[] = [{
        role: 'system',
        content: 'You are Foundry Copilot — a coding assistant running on Microsoft Foundry. Be concise and accurate.',
    }];
    // Re-play prior turns (history) so the model has context.
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
