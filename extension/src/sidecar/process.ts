// Spawns the Go sidecar binary as a child process, exposes stdin/stdout
// for an LSP-framed JSON-RPC client, and restarts on crash with backoff.
//
// The bundled binary lives at extension/bin/<os>-<arch>/foundry-copilot-sidecar
// (or .exe on Windows). The VSIX packager (Phase 10) ships one VSIX per
// platform so only the right binary is included.
import { ChildProcessWithoutNullStreams, spawn } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as vscode from 'vscode';

export interface SidecarOptions {
    extensionPath: string;
    logLevel: 'debug' | 'info' | 'warn' | 'error';
    output: vscode.OutputChannel;
}

export class Sidecar {
    private proc: ChildProcessWithoutNullStreams | undefined;
    private restarts = 0;
    private readonly opts: SidecarOptions;
    private stopping = false;
    private readonly onExitListeners = new Set<(code: number | null) => void>();

    constructor(opts: SidecarOptions) {
        this.opts = opts;
    }

    /** Resolve the bundled binary path for the current platform. */
    static binaryPath(extensionPath: string): string {
        const goos = process.platform === 'win32' ? 'windows'
            : process.platform === 'darwin' ? 'darwin'
            : 'linux';
        const goarch = process.arch === 'arm64' ? 'arm64'
            : process.arch === 'x64' ? 'amd64'
            : process.arch;
        const name = process.platform === 'win32'
            ? 'foundry-copilot-sidecar.exe'
            : 'foundry-copilot-sidecar';
        return path.join(extensionPath, 'bin', `${goos}-${goarch}`, name);
    }

    start(): void {
        const bin = Sidecar.binaryPath(this.opts.extensionPath);
        if (!fs.existsSync(bin)) {
            this.opts.output.appendLine(
                `[sidecar] binary not found at ${bin} — was the extension built with the matching Go binary?`,
            );
            throw new Error(`foundry-copilot sidecar binary not found: ${bin}`);
        }
        this.opts.output.appendLine(`[sidecar] spawning ${bin}`);
        const proc = spawn(bin, [`--log-level=${this.opts.logLevel}`], {
            stdio: ['pipe', 'pipe', 'pipe'],
            env: process.env,
        });
        this.proc = proc;

        proc.stderr.on('data', (b: Buffer) => {
            // Sidecar logs are JSON lines on stderr — surface verbatim.
            for (const line of b.toString('utf8').split(/\r?\n/)) {
                if (line.trim()) this.opts.output.appendLine(`[sidecar] ${line}`);
            }
        });

        proc.on('exit', (code, signal) => {
            this.opts.output.appendLine(`[sidecar] exited code=${code} signal=${signal}`);
            for (const l of this.onExitListeners) l(code);
            this.proc = undefined;
            if (this.stopping) return;
            // Crash-restart with linear backoff, max 5 quick restarts.
            if (this.restarts++ < 5) {
                const delay = Math.min(500 + this.restarts * 500, 5000);
                this.opts.output.appendLine(`[sidecar] restarting in ${delay}ms (attempt ${this.restarts})`);
                setTimeout(() => this.start(), delay);
            } else {
                this.opts.output.appendLine('[sidecar] too many restarts, giving up');
                vscode.window.showErrorMessage(
                    'Foundry Copilot sidecar crashed repeatedly. Check the output channel.',
                );
            }
        });

        proc.on('error', (err) => {
            this.opts.output.appendLine(`[sidecar] spawn error: ${err.message}`);
        });
    }

    stop(): void {
        this.stopping = true;
        if (this.proc) {
            this.proc.kill();
            this.proc = undefined;
        }
    }

    /** Stdio streams for use by the JSON-RPC client. Throws if not started. */
    get stdin(): NodeJS.WritableStream {
        if (!this.proc) throw new Error('sidecar not running');
        return this.proc.stdin;
    }
    get stdout(): NodeJS.ReadableStream {
        if (!this.proc) throw new Error('sidecar not running');
        return this.proc.stdout;
    }

    onExit(cb: (code: number | null) => void): vscode.Disposable {
        this.onExitListeners.add(cb);
        return { dispose: () => this.onExitListeners.delete(cb) };
    }
}

// platformId returns a human-readable id like "darwin-arm64" used in logs.
export function platformId(): string {
    return `${process.platform}-${process.arch} (node ${process.version}, ${os.release()})`;
}
