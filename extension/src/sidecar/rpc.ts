// Minimal typed JSON-RPC 2.0 client over LSP-framed stdio.
// The sidecar uses creachadair/jrpc2 with channel.LSP framing, which is
// the same Content-Length: N\r\n\r\n{body} format used by LSP.
//
// We implement just enough framing/correlation to drive the sidecar. No
// notifications from server → client are needed in Phase 2; Phase 3 adds them.
import * as vscode from 'vscode';

type Pending = {
    resolve: (value: unknown) => void;
    reject: (err: Error) => void;
    method: string;
};

export class RpcClient {
    private nextId = 1;
    private pending = new Map<number, Pending>();
    private readBuf = Buffer.alloc(0);
    private contentLength = -1;
    private readonly output: vscode.OutputChannel;
    private stdin: NodeJS.WritableStream | undefined;
    private stdout: NodeJS.ReadableStream | undefined;

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
        // Fail any outstanding requests so callers don't hang forever.
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

    /** Fire-and-forget notification (jsonrpc with no id). Phase 3 uses this. */
    notify(method: string, params?: unknown): void {
        if (!this.stdin) throw new Error('rpc not attached');
        const body = JSON.stringify({ jsonrpc: '2.0', method, params });
        const frame = `Content-Length: ${Buffer.byteLength(body, 'utf8')}\r\n\r\n${body}`;
        this.stdin.write(frame);
    }

    // ─── framing ─────────────────────────────────────────────────────────

    private onData(chunk: Buffer): void {
        this.readBuf = Buffer.concat([this.readBuf, chunk]);
        // Loop because multiple frames can arrive in one chunk.
        while (true) {
            if (this.contentLength < 0) {
                const headerEnd = this.readBuf.indexOf('\r\n\r\n');
                if (headerEnd < 0) return; // wait for more
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
            if (this.readBuf.length < this.contentLength) return; // wait
            const body = this.readBuf.slice(0, this.contentLength).toString('utf8');
            this.readBuf = this.readBuf.slice(this.contentLength);
            this.contentLength = -1;
            this.handleMessage(body);
        }
    }

    private handleMessage(body: string): void {
        let msg: { id?: number; result?: unknown; error?: { code: number; message: string } };
        try {
            msg = JSON.parse(body);
        } catch (e) {
            this.output.appendLine(`[rpc] bad json: ${body}`);
            return;
        }
        if (typeof msg.id !== 'number') {
            // Notification from server → client. Phase 3 wires a dispatcher here.
            this.output.appendLine(`[rpc] unhandled notification: ${body}`);
            return;
        }
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

// ─── typed wrappers for the methods we know ───────────────────────────────

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
};
