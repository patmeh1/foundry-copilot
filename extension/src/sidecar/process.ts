// Spawns the Go sidecar binary as a child process, exposes stdin/stdout
// for an LSP-framed JSON-RPC client, and supervises it with exponential
// backoff + circuit breaker.
//
// v0.2 supervisor contract:
//   * Exponential backoff: 1s, 2s, 4s, 8s, 16s, then capped at 30s.
//   * Circuit breaker: if the process crashes 5 times within a 60s window
//     we OPEN the breaker for 60s. While OPEN, no new spawn attempts run
//     and the status bar reports red ("sidecar disabled").
//   * Tri-color status: emits "ok" (green), "restarting" (yellow), or
//     "disabled" (red) via the onStatus event. extension.ts subscribes
//     and reflects this on the BAA + sidecar status items.
//
// The bundled binary lives at extension/bin/<os>-<arch>/foundry-copilot-sidecar
// (or .exe on Windows). The VSIX packager (Phase 10) ships one VSIX per
// platform so only the right binary is included.
import { ChildProcessWithoutNullStreams, spawn } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';
import * as vscode from 'vscode';

import {
    BREAKER_FAILURE_THRESHOLD,
    BREAKER_OPEN_MS,
    BREAKER_WINDOW_MS,
    computeBackoffMs,
    shouldOpenBreaker,
} from './supervisor';

export {
    BREAKER_FAILURE_THRESHOLD,
    BREAKER_OPEN_MS,
    BREAKER_WINDOW_MS,
} from './supervisor';

export interface SidecarOptions {
    extensionPath: string;
    logLevel: 'debug' | 'info' | 'warn' | 'error';
    output: vscode.OutputChannel;
}

export type SidecarStatus = 'ok' | 'restarting' | 'disabled';

interface CrashRecord {
    at: number;
}

// Breaker tuning is imported from ./supervisor for use in this module.

export class Sidecar {
    private proc: ChildProcessWithoutNullStreams | undefined;
    private restarts = 0;
    private readonly opts: SidecarOptions;
    private stopping = false;
    private readonly onExitListeners = new Set<(code: number | null) => void>();
    private readonly onStatusListeners = new Set<(status: SidecarStatus, reason: string) => void>();
    private readonly onAttachListeners = new Set<() => void>();
    private crashes: CrashRecord[] = [];
    private breakerOpenUntil = 0;
    private restartTimer: NodeJS.Timeout | undefined;
    private status: SidecarStatus = 'restarting';

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

    /**
     * computeBackoffMs returns the backoff in ms for the given attempt
     * count (1-indexed). Sequence: 1s, 2s, 4s, 8s, 16s, 30s capped.
     */
    static computeBackoffMs(attempt: number): number {
        return computeBackoffMs(attempt);
    }

    /** Current supervisor status. */
    getStatus(): SidecarStatus { return this.status; }

    private setStatus(s: SidecarStatus, reason: string): void {
        this.status = s;
        for (const l of this.onStatusListeners) {
            try { l(s, reason); } catch { /* swallow listener errors */ }
        }
    }

    private recordCrash(): void {
        const now = Date.now();
        this.crashes.push({ at: now });
        // Drop crashes outside the window.
        this.crashes = this.crashes.filter((c) => now - c.at <= BREAKER_WINDOW_MS);
    }

    private breakerOpen(): boolean {
        return Date.now() < this.breakerOpenUntil;
    }

    private maybeOpenBreaker(): boolean {
        if (shouldOpenBreaker(this.crashes.length)) {
            this.breakerOpenUntil = Date.now() + BREAKER_OPEN_MS;
            this.crashes = [];
            this.setStatus('disabled',
                `sidecar crashed ${BREAKER_FAILURE_THRESHOLD} times in ${BREAKER_WINDOW_MS / 1000}s — breaker OPEN for ${BREAKER_OPEN_MS / 1000}s`);
            return true;
        }
        return false;
    }

    start(): void {
        if (this.restartTimer) {
            clearTimeout(this.restartTimer);
            this.restartTimer = undefined;
        }
        if (this.breakerOpen()) {
            const wait = this.breakerOpenUntil - Date.now();
            this.opts.output.appendLine(`[sidecar] breaker OPEN; not spawning (retry in ${wait}ms)`);
            this.restartTimer = setTimeout(() => this.start(), wait);
            return;
        }
        const bin = Sidecar.binaryPath(this.opts.extensionPath);
        if (!fs.existsSync(bin)) {
            this.opts.output.appendLine(
                `[sidecar] binary not found at ${bin} — was the extension built with the matching Go binary?`,
            );
            this.setStatus('disabled', `binary missing: ${bin}`);
            throw new Error(`foundry-copilot sidecar binary not found: ${bin}`);
        }
        this.opts.output.appendLine(`[sidecar] spawning ${bin}`);
        let proc: ChildProcessWithoutNullStreams;
        try {
            proc = spawn(bin, [`--log-level=${this.opts.logLevel}`], {
                stdio: ['pipe', 'pipe', 'pipe'],
                env: process.env,
            });
        } catch (err: unknown) {
            const m = err instanceof Error ? err.message : String(err);
            this.opts.output.appendLine(`[sidecar] spawn threw: ${m}`);
            this.recordCrash();
            this.scheduleRestart('spawn threw');
            return;
        }
        this.proc = proc;
        // Notify listeners that the process is up so the RPC client can attach.
        for (const l of this.onAttachListeners) {
            try { l(); } catch { /* swallow listener errors */ }
        }
        // Spawn succeeded; mark OK now. If the process crashes immediately
        // we'll re-enter restarting on 'exit'.
        this.setStatus('ok', 'sidecar running');

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
            this.recordCrash();
            if (this.maybeOpenBreaker()) {
                this.restartTimer = setTimeout(() => this.start(), BREAKER_OPEN_MS);
                return;
            }
            this.scheduleRestart(`exit code=${code} signal=${signal}`);
        });

        proc.on('error', (err) => {
            this.opts.output.appendLine(`[sidecar] spawn error: ${err.message}`);
        });
    }

    private scheduleRestart(reason: string): void {
        this.restarts++;
        const delay = computeBackoffMs(this.restarts);
        this.opts.output.appendLine(`[sidecar] restarting in ${delay}ms (attempt ${this.restarts}): ${reason}`);
        this.setStatus('restarting', reason);
        this.restartTimer = setTimeout(() => this.start(), delay);
    }

    stop(): void {
        this.stopping = true;
        if (this.restartTimer) {
            clearTimeout(this.restartTimer);
            this.restartTimer = undefined;
        }
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

    onStatus(cb: (status: SidecarStatus, reason: string) => void): vscode.Disposable {
        this.onStatusListeners.add(cb);
        return { dispose: () => this.onStatusListeners.delete(cb) };
    }

    onAttach(cb: () => void): vscode.Disposable {
        this.onAttachListeners.add(cb);
        return { dispose: () => this.onAttachListeners.delete(cb) };
    }

    /**
     * Reset the breaker manually (called by the
     * `foundryCopilot.sidecar.resetBreaker` command).
     */
    resetBreaker(): void {
        this.crashes = [];
        this.breakerOpenUntil = 0;
        this.restarts = 0;
        this.opts.output.appendLine('[sidecar] breaker manually reset');
        this.start();
    }
}

// platformId returns a human-readable id like "darwin-arm64" used in logs.
export function platformId(): string {
    return `${process.platform}-${process.arch} (node ${process.version}, ${os.release()})`;
}
