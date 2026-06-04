// Activation entry point. Activation order (v0.2):
//   1. BAA guard FIRST (must run before any sidecar/network so conflicting
//      GitHub Copilot extensions are disabled before they can intercept LM
//      API calls).
//   2. Sidecar spawn + RPC attach.
//   3. LanguageModelChatProvider registration (if proposed API present).
//   4. Foundry chat view + threads + diff surface.
//   5. Deployments / telemetry / finetune / billing tree views.
//   6. Inline / quick / terminal / notebook chat commands.
//   7. SCM (commit msg, PR desc), tests diagnose, instructions migration.
//   8. NES inline-completion provider (gated).
//   9. Team config watcher (.foundry/**).
import * as vscode from 'vscode';
import { registerChatParticipant } from './chat/participant';
import { registerInlineCompletionProvider } from './completions/provider';
import { Methods, RpcClient, SidecarConfig } from './sidecar/rpc';
import { Sidecar, platformId } from './sidecar/process';
import { BaaStatusController } from './baa/status';
import { registerLmProvider } from './lm/provider';
import { registerChatView } from './views/chat/container';
import { registerDeploymentsView } from './views/deployments';
import { registerTelemetryView } from './views/telemetry';
import { registerFinetuneView } from './views/finetune';
import { registerBillingView } from './views/billing';
import { registerInlineChat } from './inline/inline-chat';
import { registerQuickChat } from './inline/quick-chat';
import { registerTerminalChat } from './inline/terminal-chat';
import { registerNotebookChat } from './notebook/notebook-chat';
import { registerCommitMessage } from './scm/commit-message';
import { registerPrDescription } from './scm/pr-description';
import { registerTestsDiagnose } from './tests/diagnose';
import { registerNesProvider } from './nes/provider';
import { maybePromptCopilotMigration } from './instructions/loader';
import { registerTeamWatcher } from './team/loader';

const CFG_NS = 'foundryCopilot';

let sidecar: Sidecar | undefined;
let rpc: RpcClient | undefined;
let output: vscode.OutputChannel | undefined;
let baa: BaaStatusController | undefined;

export async function activate(context: vscode.ExtensionContext): Promise<void> {
    output = vscode.window.createOutputChannel('Foundry Copilot');
    output.appendLine(`[ext] activating on ${platformId()}`);
    context.subscriptions.push(output);

    // 1. BAA guard MUST run before anything else.
    baa = new BaaStatusController(output);
    await baa.start(context);

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

    // Sidecar tri-color status bar: green=running, yellow=restarting, red=disabled.
    const statusBar = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    statusBar.command = 'foundryCopilot.showOutput';
    statusBar.text = '$(circle-filled) Foundry';
    statusBar.tooltip = 'Foundry Copilot sidecar — click for output';
    statusBar.show();
    context.subscriptions.push(statusBar);
    context.subscriptions.push(
        sidecar.onStatus((status, reason) => {
            if (status === 'disabled') {
                statusBar.text = '$(error) Foundry';
                statusBar.tooltip = `Foundry sidecar disabled: ${reason}`;
                statusBar.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
            } else if (status === 'restarting') {
                statusBar.text = '$(sync~spin) Foundry';
                statusBar.tooltip = `Foundry sidecar restarting: ${reason}`;
                statusBar.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
            } else {
                statusBar.text = '$(check) Foundry';
                statusBar.tooltip = `Foundry sidecar running (${reason || 'ok'})`;
                statusBar.backgroundColor = undefined;
            }
        }),
    );

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
    await reconnectMcpServers();

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

    // v0.2 Phase 11: LanguageModelChatProvider (proposed API; gated).
    try {
        registerLmProvider(context, rpc, output);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] LM provider registration failed: ${m}`);
    }

    // v0.2 Phase 12: Foundry chat view (BAA-safe replacement for Copilot Chat).
    try {
        registerChatView(context, rpc, output);
        output.appendLine('[ext] Foundry chat view registered');
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] chat view registration failed: ${m}`);
    }

    // v0.2 Phases 14-18: tree views (deployments / telemetry / finetune / billing).
    try {
        registerDeploymentsView(context, rpc);
        registerTelemetryView(context, rpc);
        registerFinetuneView(context, rpc);
        registerBillingView(context, rpc);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] view registration failed: ${m}`);
    }

    // v0.2 Phase 12: inline / quick / terminal / notebook chat commands.
    try {
        registerInlineChat(context, rpc, output);
        registerQuickChat(context, rpc, output);
        registerTerminalChat(context, rpc, output);
        registerNotebookChat(context, rpc, output);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] inline/notebook command registration failed: ${m}`);
    }

    // v0.2 Phase 13: SCM, tests diagnose, NES.
    try {
        registerCommitMessage(context, rpc, output);
        registerPrDescription(context, rpc, output);
        registerTestsDiagnose(context, rpc, output);
        registerNesProvider(context, rpc, output);
    } catch (err: unknown) {
        const m = err instanceof Error ? err.message : String(err);
        output.appendLine(`[ext] scm/tests/nes registration failed: ${m}`);
    }

    // v0.2 Phase 13/16: workspace instructions migration + team config watcher.
    const wsFolder = vscode.workspace.workspaceFolders?.[0];
    const outChan = output;
    if (wsFolder) {
        try {
            await maybePromptCopilotMigration(context, wsFolder.uri.fsPath, outChan);
        } catch (err: unknown) {
            const m = err instanceof Error ? err.message : String(err);
            outChan.appendLine(`[ext] instructions migration check failed: ${m}`);
        }
        registerTeamWatcher(context, outChan, () => {
            outChan.appendLine('[ext] team config changed; reloading sidecar settings');
            void pushSettings();
        });
    }

    context.subscriptions.push(
        vscode.workspace.onDidChangeConfiguration((e) => {
            if (e.affectsConfiguration(CFG_NS)) {
                void pushSettings();
            }
            if (e.affectsConfiguration(CFG_NS + '.mcp.servers')) {
                void reconnectMcpServers();
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
        vscode.commands.registerCommand('foundryCopilot.reconnectMcp', async () => {
            await reconnectMcpServers();
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

interface McpServerSetting {
    id: string;
    command: string;
    args?: string[];
    env?: Record<string, string>;
}

async function reconnectMcpServers(): Promise<void> {
    if (!rpc) return;
    const cfg = vscode.workspace.getConfiguration(CFG_NS);
    const servers = cfg.get<McpServerSetting[]>('mcp.servers', []);
    // Disconnect any previously connected servers not in the new list.
    try {
        const live = await Methods.mcpList(rpc);
        const wanted = new Set(servers.map((s) => s.id));
        for (const id of live.servers) {
            if (!wanted.has(id)) {
                try { await Methods.mcpDisconnect(rpc, { id }); }
                catch { /* ignore */ }
            }
        }
    } catch { /* sidecar may not be ready yet */ }
    // Connect (or reconnect) everything in the desired list.
    for (const spec of servers) {
        if (!spec || !spec.id || !spec.command) continue;
        try {
            const r = await Methods.mcpConnect(rpc, {
                id: spec.id, command: spec.command,
                args: spec.args ?? [], env: spec.env ?? {},
            });
            if (output) output.appendLine(`[ext] mcp connect ${r.id}: ${r.tools} tools`);
        } catch (err: unknown) {
            const m = err instanceof Error ? err.message : String(err);
            if (output) output.appendLine(`[ext] mcp connect ${spec.id} failed: ${m}`);
        }
    }
}
