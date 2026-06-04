// Minimal typed JSON-RPC 2.0 client over LSP-framed stdio.
// The sidecar uses creachadair/jrpc2 with channel.LSP framing, which is
// the same Content-Length: N\r\n\r\n{body} format used by LSP.
import * as vscode from 'vscode';

type Pending = {
    resolve: (value: unknown) => void;
    reject: (err: Error) => void;
    method: string;
};

export type NotificationHandler = (params: unknown) => void;

export class RpcClient {
    private nextId = 1;
    private pending = new Map<number, Pending>();
    private readBuf = Buffer.alloc(0);
    private contentLength = -1;
    private readonly output: vscode.OutputChannel;
    private stdin: NodeJS.WritableStream | undefined;
    private stdout: NodeJS.ReadableStream | undefined;
    private notificationHandlers = new Map<string, Set<NotificationHandler>>();

    constructor(output: vscode.OutputChannel) {
        this.output = output;
    }

    attach(stdin: NodeJS.WritableStream, stdout: NodeJS.ReadableStream): void {
        this.detach();
        this.stdin = stdin;
        this.stdout = stdout;
        stdout.on('data', (b: Buffer) => this.onData(b));
        stdout.on('error', (err: Error) => this.output.appendLine(`[rpc] stdout error: ${err.message}`));
    }

    detach(): void {
        this.stdin = undefined;
        this.stdout = undefined;
        this.readBuf = Buffer.alloc(0);
        this.contentLength = -1;
        for (const [, p] of this.pending) {
            p.reject(new Error('sidecar disconnected'));
        }
        this.pending.clear();
    }

    /** Send a request and await the result. Generic R = expected result type. */
    async request<R = unknown>(method: string, params?: unknown): Promise<R> {
        if (!this.stdin) throw new Error('rpc not attached');
        const id = this.nextId++;
        const body = JSON.stringify({ jsonrpc: '2.0', id, method, params });
        const frame = `Content-Length: ${Buffer.byteLength(body, 'utf8')}\r\n\r\n${body}`;
        return new Promise<R>((resolve, reject) => {
            this.pending.set(id, {
                resolve: (v) => resolve(v as R),
                reject,
                method,
            });
            this.stdin!.write(frame, (err) => {
                if (err) {
                    this.pending.delete(id);
                    reject(err);
                }
            });
        });
    }

    /** Fire-and-forget notification (jsonrpc with no id). */
    notify(method: string, params?: unknown): void {
        if (!this.stdin) throw new Error('rpc not attached');
        const body = JSON.stringify({ jsonrpc: '2.0', method, params });
        const frame = `Content-Length: ${Buffer.byteLength(body, 'utf8')}\r\n\r\n${body}`;
        this.stdin.write(frame);
    }

    /** Subscribe to a server → client notification. Returns a disposer. */
    onNotification(method: string, handler: NotificationHandler): vscode.Disposable {
        let set = this.notificationHandlers.get(method);
        if (!set) {
            set = new Set();
            this.notificationHandlers.set(method, set);
        }
        set.add(handler);
        return { dispose: () => set!.delete(handler) };
    }

    // ─── framing ─────────────────────────────────────────────────────────

    private onData(chunk: Buffer): void {
        this.readBuf = Buffer.concat([this.readBuf, chunk]);
        while (true) {
            if (this.contentLength < 0) {
                const headerEnd = this.readBuf.indexOf('\r\n\r\n');
                if (headerEnd < 0) return;
                const header = this.readBuf.slice(0, headerEnd).toString('utf8');
                const m = /Content-Length:\s*(\d+)/i.exec(header);
                if (!m) {
                    this.output.appendLine(`[rpc] malformed header: ${header}`);
                    this.readBuf = this.readBuf.slice(headerEnd + 4);
                    continue;
                }
                this.contentLength = parseInt(m[1], 10);
                this.readBuf = this.readBuf.slice(headerEnd + 4);
            }
            if (this.readBuf.length < this.contentLength) return;
            const body = this.readBuf.slice(0, this.contentLength).toString('utf8');
            this.readBuf = this.readBuf.slice(this.contentLength);
            this.contentLength = -1;
            this.handleMessage(body);
        }
    }

    private handleMessage(body: string): void {
        let msg: {
            id?: number;
            method?: string;
            params?: unknown;
            result?: unknown;
            error?: { code: number; message: string };
        };
        try {
            msg = JSON.parse(body);
        } catch (e) {
            this.output.appendLine(`[rpc] bad json: ${body}`);
            return;
        }
        if (typeof msg.id !== 'number' && typeof msg.method === 'string') {
            const set = this.notificationHandlers.get(msg.method);
            if (set) {
                for (const h of set) {
                    try { h(msg.params); }
                    catch (e) { this.output.appendLine(`[rpc] notif handler error: ${(e as Error).message}`); }
                }
            } else {
                this.output.appendLine(`[rpc] unhandled notification: ${msg.method}`);
            }
            return;
        }
        if (typeof msg.id !== 'number') return;
        const pending = this.pending.get(msg.id);
        if (!pending) {
            this.output.appendLine(`[rpc] reply with unknown id ${msg.id}`);
            return;
        }
        this.pending.delete(msg.id);
        if (msg.error) {
            pending.reject(new Error(`${pending.method}: ${msg.error.message} (code ${msg.error.code})`));
        } else {
            pending.resolve(msg.result);
        }
    }
}

// ─── typed types & method wrappers ────────────────────────────────────────

export interface PingReply {
    ok: boolean;
    version: string;
}

export interface SidecarConfig {
    endpoint?: string;
    chat_deployment?: string;
    completion_deployment?: string;
    embedding_deployment?: string;
    log_level?: string;
    agent_max_steps?: number;
    agent_allow_shell?: boolean;
    agent_allow_write?: boolean;
    workspace_root?: string;
}

export interface ChatMessage {
    role: 'system' | 'user' | 'assistant' | 'tool';
    content: string;
    name?: string;
    tool_id?: string;
}

export interface ChatStartParams {
    stream_id: string;
    deployment?: string;
    messages: ChatMessage[];
    temperature?: number;
    max_tokens?: number;
}

export interface ChatStartReply {
    stream_id: string;
    finish_reason?: string;
}

export interface ChatChunk {
    stream_id: string;
    delta?: string;
    finish_reason?: string;
    error?: string;
}

export interface AgentRunParams {
    stream_id: string;
    task: string;
    workdir?: string;
    system?: string;
    max_steps?: number;
}
export interface AgentRunReply {
    stream_id: string;
    final_message: string;
    steps_taken: number;
}
export interface AgentEvent {
    stream_id: string;
    kind: 'think' | 'tool_call' | 'tool_result' | 'final' | 'error';
    step: number;
    text?: string;
    tool?: string;
    args?: string;
    result?: string;
    tool_error?: string;
}

export interface IndexRefreshParams {
    paths?: string[];
    max_files?: number;
}
export interface IndexRefreshReply {
    files: number;
    chunks: number;
    skipped: number;
}
export interface IndexQueryParams {
    query: string;
    k?: number;
}
export interface IndexHit {
    path: string;
    start_line: number;
    end_line: number;
    text: string;
    score: number;
}
export interface IndexQueryReply {
    hits: IndexHit[];
}

export const Methods = {
    ping(rpc: RpcClient): Promise<PingReply> {
        return rpc.request<PingReply>('ping');
    },
    version(rpc: RpcClient): Promise<string> {
        return rpc.request<string>('version');
    },
    configSet(rpc: RpcClient, cfg: SidecarConfig): Promise<SidecarConfig> {
        return rpc.request<SidecarConfig>('config/set', cfg);
    },
    configGet(rpc: RpcClient): Promise<SidecarConfig> {
        return rpc.request<SidecarConfig>('config/get');
    },
    chatStart(rpc: RpcClient, p: ChatStartParams): Promise<ChatStartReply> {
        return rpc.request<ChatStartReply>('chat/start', p);
    },
    agentRun(rpc: RpcClient, p: AgentRunParams): Promise<AgentRunReply> {
        return rpc.request<AgentRunReply>('agent/run', p);
    },
    indexRefresh(rpc: RpcClient, p: IndexRefreshParams = {}): Promise<IndexRefreshReply> {
        return rpc.request<IndexRefreshReply>('index/refresh', p);
    },
    indexQuery(rpc: RpcClient, p: IndexQueryParams): Promise<IndexQueryReply> {
        return rpc.request<IndexQueryReply>('index/query', p);
    },
    mcpConnect(rpc: RpcClient, p: MCPConnectParams): Promise<MCPConnectReply> {
        return rpc.request<MCPConnectReply>('mcp/connect', p);
    },
    mcpDisconnect(rpc: RpcClient, p: { id: string }): Promise<{ ok: boolean }> {
        return rpc.request<{ ok: boolean }>('mcp/disconnect', p);
    },
    mcpList(rpc: RpcClient): Promise<MCPListReply> {
        return rpc.request<MCPListReply>('mcp/list');
    },
    mcpListTools(rpc: RpcClient, p: { id?: string } = {}): Promise<MCPListToolsReply> {
        return rpc.request<MCPListToolsReply>('mcp/list_tools', p);
    },
    mcpCallTool(rpc: RpcClient, p: MCPCallToolParams): Promise<MCPCallToolReply> {
        return rpc.request<MCPCallToolReply>('mcp/call_tool', p);
    },
};

export interface MCPConnectParams {
    id: string;
    command: string;
    args?: string[];
    env?: Record<string, string>;
}
export interface MCPConnectReply {
    id: string;
    tools: number;
}
export interface MCPListReply {
    servers: string[];
}
export interface MCPListToolsTool {
    server_id: string;
    name: string;
    description: string;
    input_schema: Record<string, unknown>;
}
export interface MCPListToolsReply {
    tools: MCPListToolsTool[];
}
export interface MCPCallToolParams {
    id: string;
    name: string;
    args?: Record<string, unknown>;
}
export interface MCPCallToolReply {
    text: string;
    error?: string;
}
