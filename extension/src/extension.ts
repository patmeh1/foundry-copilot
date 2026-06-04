// Activation entry point. Spawns the sidecar, attaches an RPC client,
// pushes VS Code settings into the sidecar, and exposes diagnostic
// commands. Phase 3 will register the chat participant here.
import * as vscode from 'vscode';
import { registerChatParticipant } from './chat/participant';
import { registerInlineCompletionProvider } from './completions/provider';
import { Methods, RpcClient, SidecarConfig } from './sidecar/rpc';
import { Sidecar, platformId } from './sidecar/process';

const CFG_NS = 'foundryCopilot';

let sidecar: Sidecar | undefined;
let rpc: RpcClient | undefined;
let output: vscode.OutputChannel | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
    output = vscode.window.createOutputChannel('Foundry Copilot');
    output.appendLine(`[ext] activating on ${platformId()}`);
    context.subscriptions.push(output);

    const logLevel = readLogLevel();
    sidecar = new Sidecar({
        extensionPath: context.extensionPath,
        logLevel,
        output,
    });
    rpc = new RpcClient(output);

    try {
        sidecar.start();
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        vscode.window.showErrorMessage(`Foundry Copilot: ${m}`);
        return;
    }
    rpc.attach(sidecar.stdin, sidecar.stdout);

    context.subscriptions.push(
        sidecar.onExit(() => {
            if (rpc) rpc.detach();
            // Sidecar.start() handles restart; re-attach on next spawn via the
            // 'spawn' event isn't wired — keep it simple: restart goes through
            // setTimeout in Sidecar, so we hook the next stdin via a poll-ish
            // re-attach below.
            const retry = setInterval(() => {
                try {
                    rpc!.attach(sidecar!.stdin, sidecar!.stdout);
                    clearInterval(retry);
                    void pushSettings();
                } catch {
                    /* not ready yet */
                }
            }, 250);
            // Stop trying after 30s.
            setTimeout(() => clearInterval(retry), 30_000);
        }),
    );

    // Ping to confirm liveness; non-fatal if it times out.
    try {
        const reply = await withTimeout(Methods.ping(rpc), 5_000);
        output.appendLine(`[ext] sidecar ping ok, version=${reply.version}`);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] sidecar ping failed: ${m}`);
    }

    await pushSettings();

    // Phase 3: register the @foundry chat participant.
    try {
        registerChatParticipant(context, rpc, output);
        output.appendLine('[ext] chat participant @foundry registered');
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] chat participant registration failed: ${m}`);
    }

    // Phase 4: inline completions.
    try {
        registerInlineCompletionProvider(context, rpc, output);
        output.appendLine('[ext] inline completion provider registered');
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] inline completion registration failed: ${m}`);
    }

    context.subscriptions.push(
        vscode.workspace.onDidChangeConfiguration((e) => {
            if (e.affectsConfiguration(CFG_NS)) {
                void pushSettings();
            }
        }),
        vscode.commands.registerCommand('foundryCopilot.ping', async () => {
            try {
                const r = await Methods.ping(rpc!);
                vscode.window.showInformationMessage(`Foundry Copilot sidecar OK, v${r.version}`);
            } catch (err: unknown) {
                const m = err instanceof Error ? err.message : String(err);
                vscode.window.showErrorMessage(`Sidecar ping failed: ${m}`);
            }
        }),
        vscode.commands.registerCommand('foundryCopilot.showOutput', () => output!.show()),
        vscode.commands.registerCommand('foundryCopilot.refreshIndex', async () => {
            if (!rpc) {
                vscode.window.showErrorMessage('Foundry Copilot: sidecar not ready');
                return;
            }
            await vscode.window.withProgress(
                {
                    location: vscode.ProgressLocation.Notification,
                    title: 'Foundry Copilot: indexing workspace…',
                    cancellable: false,
                },
                async (progress) => {
                    progress.report({ message: 'walking files & embedding (this can take a minute)' });
                    try {
                        const r = await Methods.indexRefresh(rpc!, {});
                        vscode.window.showInformationMessage(
                            `Indexed ${r.files} files → ${r.chunks} chunks (skipped ${r.skipped}).`,
                        );
                    } catch (err: unknown) {
                        const m = err instanceof Error ? err.message : String(err);
                        vscode.window.showErrorMessage(`Index refresh failed: ${m}`);
                    }
                },
            );
        }),
    );
}

export function deactivate(): void {
    if (rpc) rpc.detach();
    if (sidecar) sidecar.stop();
}

// ─── helpers ───────────────────────────────────────────────────────────────

function readLogLevel(): 'debug' | 'info' | 'warn' | 'error' {
    const v = vscode.workspace.getConfiguration(CFG_NS).get<string>('logLevel', 'info');
    if (v === 'debug' || v === 'info' || v === 'warn' || v === 'error') return v;
    return 'info';
}

async function pushSettings(): Promise<void> {
    if (!rpc) return;
    const cfg = vscode.workspace.getConfiguration(CFG_NS);
    const folder = vscode.workspace.workspaceFolders?.[0];
    const payload: SidecarConfig = {
        endpoint: cfg.get<string>('endpoint', ''),
        chat_deployment: cfg.get<string>('chatDeployment', ''),
        completion_deployment: cfg.get<string>('completionDeployment', ''),
        embedding_deployment: cfg.get<string>('embeddingDeployment', ''),
        log_level: cfg.get<string>('logLevel', 'info'),
        agent_max_steps: cfg.get<number>('agent.maxSteps', 12),
        agent_allow_shell: cfg.get<boolean>('agent.allowShell', false),
        agent_allow_write: cfg.get<boolean>('agent.allowWrite', false),
        workspace_root: folder ? folder.uri.fsPath : '',
    };
    try {
        await Methods.configSet(rpc, payload);
        if (output) output.appendLine(`[ext] pushed settings (endpoint=${payload.endpoint ? 'set' : 'empty'}, root=${payload.workspace_root || 'none'})`);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        if (output) output.appendLine(`[ext] config/set rejected: ${m}`);
        if (payload.endpoint && /hard lock|not a Microsoft Foundry/i.test(m)) {
            vscode.window.showErrorMessage(
                `Foundry Copilot rejected endpoint "${payload.endpoint}": ${m}`,
            );
        }
    }
}

function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
    return new Promise<T>((resolve, reject) => {
        const t = setTimeout(() => reject(new Error(`timeout after ${ms}ms`)), ms);
        p.then(
            (v) => {
                clearTimeout(t);
                resolve(v);
            },
            (err) => {
                clearTimeout(t);
                reject(err);
            },
        );
    });
}
