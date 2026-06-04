// finetune.ts — opens a webview that lists fine-tune jobs. The underlying
// JobsClient surfaces ErrNotImplemented in v0.2.0 (real wire protocol
// lands in v0.2.1) — the panel renders the error inline rather than
// hiding the feature.
import * as vscode from 'vscode';
import { FinetuneJob, Methods, RpcClient } from '../sidecar/rpc';

export function registerFinetuneView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.finetune.open', async () => {
            const panel = vscode.window.createWebviewPanel(
                'foundryCopilot.finetune',
                'Foundry Fine-Tuning',
                vscode.ViewColumn.Active,
                { enableScripts: true },
            );
            const refresh = async () => {
                try {
                    const jobs = await Methods.finetuneListJobs(rpc);
                    panel.webview.html = renderHtml(jobs, undefined);
                } catch (err) {
                    panel.webview.html = renderHtml([], (err as Error).message);
                }
            };
            panel.webview.onDidReceiveMessage((m) => {
                if (m?.type === 'refresh') void refresh();
            });
            await refresh();
        }),
    );
}

function renderHtml(jobs: FinetuneJob[], error: string | undefined): string {
    const rows = jobs.length
        ? jobs.map((j) => `<tr><td>${escape(j.id)}</td><td>${escape(j.model)}</td><td>${escape(j.status)}</td><td>${escape(j.created_at ?? '')}</td></tr>`).join('')
        : `<tr><td colspan="4" class="muted">${error ? escape(error) : 'No jobs.'}</td></tr>`;
    return `<!doctype html><html><head><meta charset="utf-8"/><style>
body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); padding: 12px; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 4px 8px; text-align: left; border-bottom: 1px solid var(--vscode-panel-border); font-size: 12px; }
.muted { opacity: 0.6; font-style: italic; }
button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; padding: 4px 10px; cursor: pointer; }
</style></head><body>
<h2>Foundry Fine-Tuning Jobs</h2>
<p><button id="r">Refresh</button></p>
<table><thead><tr><th>ID</th><th>Model</th><th>Status</th><th>Created</th></tr></thead><tbody>${rows}</tbody></table>
<script>
const v = acquireVsCodeApi();
document.getElementById('r').addEventListener('click', () => v.postMessage({ type: 'refresh' }));
</script>
</body></html>`;
}

function escape(s: string): string {
    return s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c] ?? c));
}
