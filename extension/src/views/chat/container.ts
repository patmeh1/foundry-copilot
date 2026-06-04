// container.ts — the Foundry-native chat view that replaces the GitHub
// Copilot Chat side-panel. v0.2.0 ships the real streaming surface: each
// user message is sent through chat/start with a unique stream_id, and
// incoming chat/chunk notifications are appended to the live assistant
// message until finish_reason fires. No GitHub Copilot Chat dependency.
import * as vscode from 'vscode';
import { ChatMessage, Methods, RpcClient } from '../../sidecar/rpc';
import { ChatThread, ThreadStore } from './threads';

const VIEW_ID = 'foundryCopilot.chatView';

interface ChunkPayload {
    stream_id: string;
    delta?: string;
    finish_reason?: string;
    error?: string;
}

export class FoundryChatViewProvider implements vscode.WebviewViewProvider {
    private view: vscode.WebviewView | undefined;
    private currentThreadId: string | undefined;
    private activeStreams = new Map<string, { threadId: string; buffer: string }>();
    private chunkSub: vscode.Disposable | undefined;

    constructor(
        private readonly context: vscode.ExtensionContext,
        private readonly rpc: RpcClient,
        private readonly output: vscode.OutputChannel,
        private readonly threads: ThreadStore,
    ) {
        this.chunkSub = this.rpc.onNotification('chat/chunk', (params) => this.onChunk(params as ChunkPayload));
        this.context.subscriptions.push(this.chunkSub);
    }

    resolveWebviewView(webviewView: vscode.WebviewView): void {
        this.view = webviewView;
        webviewView.webview.options = { enableScripts: true, localResourceRoots: [this.context.extensionUri] };
        webviewView.webview.html = this.renderHtml(webviewView.webview);
        webviewView.webview.onDidReceiveMessage((msg) => this.onMessage(msg));
        this.postThreads();
    }

    private async onMessage(msg: { type: string; payload?: any }): Promise<void> {
        switch (msg.type) {
            case 'newThread': {
                const t = this.threads.create(`Chat ${this.threads.list().length + 1}`);
                this.currentThreadId = t.id;
                this.postThreads();
                break;
            }
            case 'selectThread': {
                this.currentThreadId = String(msg.payload?.id ?? '');
                this.postThreads();
                break;
            }
            case 'send': {
                const text = String(msg.payload?.text ?? '').trim();
                if (!text) return;
                await this.sendUserMessage(text);
                break;
            }
            default:
                this.output.appendLine(`[chat-view] unknown message: ${msg.type}`);
        }
    }

    private async sendUserMessage(text: string): Promise<void> {
        if (!this.currentThreadId) {
            const t = this.threads.create('New chat');
            this.currentThreadId = t.id;
        }
        const thread = this.threads.get(this.currentThreadId!);
        if (!thread) return;
        thread.messages.push({ role: 'user', content: text });
        this.threads.update(thread);
        // Show the user msg + a placeholder assistant entry.
        const streamId = `${thread.id}-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
        thread.messages.push({ role: 'assistant', content: '' });
        this.threads.update(thread);
        this.activeStreams.set(streamId, { threadId: thread.id, buffer: '' });
        this.view?.webview.postMessage({ type: 'threads', payload: { threads: this.threads.list(), currentId: this.currentThreadId } });

        const messages: ChatMessage[] = thread.messages
            .slice(0, -1) // drop the empty assistant placeholder
            .map((m) => ({ role: m.role as ChatMessage['role'], content: m.content }));
        try {
            await Methods.chatStart(this.rpc, { stream_id: streamId, messages });
        } catch (err) {
            const msg = (err as Error).message;
            this.output.appendLine(`[chat-view] chat/start failed: ${msg}`);
            const live = this.activeStreams.get(streamId);
            if (live) {
                const t = this.threads.get(live.threadId);
                if (t && t.messages.length > 0) {
                    t.messages[t.messages.length - 1].content = `(error) ${msg}`;
                    this.threads.update(t);
                    this.view?.webview.postMessage({ type: 'threads', payload: { threads: this.threads.list(), currentId: this.currentThreadId } });
                }
                this.activeStreams.delete(streamId);
            }
        }
    }

    private onChunk(c: ChunkPayload): void {
        if (!c?.stream_id) return;
        const live = this.activeStreams.get(c.stream_id);
        if (!live) return;
        if (c.error) {
            const t = this.threads.get(live.threadId);
            if (t && t.messages.length > 0) {
                t.messages[t.messages.length - 1].content = `(error) ${c.error}`;
                this.threads.update(t);
            }
            this.activeStreams.delete(c.stream_id);
            this.view?.webview.postMessage({ type: 'threads', payload: { threads: this.threads.list(), currentId: this.currentThreadId } });
            return;
        }
        if (c.delta) {
            live.buffer += c.delta;
            const t = this.threads.get(live.threadId);
            if (t && t.messages.length > 0) {
                t.messages[t.messages.length - 1].content = live.buffer;
                this.threads.update(t);
                this.view?.webview.postMessage({ type: 'stream', payload: { threadId: live.threadId, text: live.buffer } });
            }
        }
        if (c.finish_reason) {
            this.activeStreams.delete(c.stream_id);
        }
    }

    private postThreads(): void {
        if (!this.view) return;
        const threads: ChatThread[] = this.threads.list();
        this.view.webview.postMessage({
            type: 'threads',
            payload: { threads, currentId: this.currentThreadId },
        });
    }

    private renderHtml(webview: vscode.Webview): string {
        const nonce = randomNonce();
        const csp = [
            "default-src 'none'",
            `style-src ${webview.cspSource} 'unsafe-inline'`,
            `script-src 'nonce-${nonce}'`,
        ].join('; ');
        return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8" />
<meta http-equiv="Content-Security-Policy" content="${csp}" />
<title>Foundry Chat</title>
<style>
  body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); background: var(--vscode-sideBar-background); margin: 0; padding: 0; display: flex; flex-direction: column; height: 100vh; }
  header { padding: 6px 10px; display: flex; gap: 8px; align-items: center; border-bottom: 1px solid var(--vscode-panel-border); }
  header select { flex: 1; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); }
  #log { flex: 1; overflow-y: auto; padding: 10px; }
  .msg { margin-bottom: 12px; }
  .role { font-size: 11px; opacity: 0.7; }
  .body { white-space: pre-wrap; margin-top: 2px; }
  footer { border-top: 1px solid var(--vscode-panel-border); padding: 6px; display: flex; gap: 4px; }
  textarea { flex: 1; resize: none; min-height: 32px; max-height: 120px; background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); padding: 4px 6px; font-family: inherit; }
  button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; padding: 4px 10px; cursor: pointer; }
</style>
</head>
<body>
  <header>
    <select id="thread"></select>
    <button id="new">+</button>
  </header>
  <div id="log"></div>
  <footer>
    <textarea id="input" placeholder="Ask Foundry…"></textarea>
    <button id="send">Send</button>
  </footer>
<script nonce="${nonce}">
const vscode = acquireVsCodeApi();
const log = document.getElementById('log');
const input = document.getElementById('input');
const send = document.getElementById('send');
const thread = document.getElementById('thread');
let threads = []; let currentId = undefined;
function render() {
  log.innerHTML = '';
  const t = threads.find(x => x.id === currentId);
  if (!t) return;
  for (const m of t.messages) {
    const d = document.createElement('div');
    d.className = 'msg';
    d.innerHTML = '<div class="role">' + m.role + '</div><div class="body"></div>';
    d.querySelector('.body').textContent = m.content;
    log.appendChild(d);
  }
  log.scrollTop = log.scrollHeight;
}
function renderThreads() {
  thread.innerHTML = '';
  for (const t of threads) {
    const o = document.createElement('option');
    o.value = t.id; o.textContent = t.title;
    if (t.id === currentId) o.selected = true;
    thread.appendChild(o);
  }
  render();
}
thread.addEventListener('change', () => vscode.postMessage({ type: 'selectThread', payload: { id: thread.value } }));
document.getElementById('new').addEventListener('click', () => vscode.postMessage({ type: 'newThread' }));
send.addEventListener('click', () => {
  const text = input.value;
  if (!text.trim()) return;
  vscode.postMessage({ type: 'send', payload: { text } });
  input.value = '';
});
input.addEventListener('keydown', (e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send.click(); } });
window.addEventListener('message', (e) => {
  const m = e.data;
  if (m.type === 'threads') { threads = m.payload.threads; currentId = m.payload.currentId; renderThreads(); }
  else if (m.type === 'stream') {
    const t = threads.find(x => x.id === m.payload.threadId);
    if (t && t.messages.length > 0) {
      t.messages[t.messages.length - 1].content = m.payload.text;
      render();
    }
  }
  else if (m.type === 'assistant') {
    const t = threads.find(x => x.id === m.payload.threadId);
    if (t) { t.messages.push({ role: 'assistant', content: m.payload.text }); render(); }
  }
});
</script>
</body>
</html>`;
    }
}

export function registerChatView(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): void {
    const threads = new ThreadStore(context);
    const provider = new FoundryChatViewProvider(context, rpc, output, threads);
    context.subscriptions.push(
        vscode.window.registerWebviewViewProvider(VIEW_ID, provider, {
            webviewOptions: { retainContextWhenHidden: true },
        }),
    );
}

function randomNonce(): string {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    let out = '';
    for (let i = 0; i < 32; i++) out += chars[Math.floor(Math.random() * chars.length)];
    return out;
}
