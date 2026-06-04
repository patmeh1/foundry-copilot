// telemetry.ts — opens a webview that shows the rolling 24h telemetry
// summary plus the last 25 error rows. Reads from telemetry/summary +
// telemetry/errors on demand; refresh button re-fetches.
import * as vscode from 'vscode';
import { Methods, RpcClient, TelemetryErrorRow, TelemetrySummary } from '../sidecar/rpc';

export function registerTelemetryView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.telemetry.open', async () => {
            const panel = vscode.window.createWebviewPanel(
                'foundryCopilot.telemetry',
                'Foundry Telemetry',
                vscode.ViewColumn.Active,
                { enableScripts: true },
            );
            const refresh = async () => {
                try {
                    const [summary, errors] = await Promise.all([
                        Methods.telemetrySummary(rpc, { window_hours: 24 }),
                        Methods.telemetryErrors(rpc, { limit: 25 }),
                    ]);
                    panel.webview.html = renderHtml(summary, errors.rows);
                } catch (err) {
                    panel.webview.html = renderError((err as Error).message);
                }
            };
            panel.webview.onDidReceiveMessage((m) => {
                if (m?.type === 'refresh') void refresh();
                if (m?.type === 'clear') {
                    void Methods.telemetryClear(rpc).then(refresh).catch((err) => {
                        vscode.window.showErrorMessage(`telemetry/clear failed: ${(err as Error).message}`);
                    });
                }
            });
            await refresh();
        }),
    );
}

function renderHtml(s: TelemetrySummary, rows: TelemetryErrorRow[]): string {
    const tableRows = rows.length
        ? rows.map((r) => `<tr><td>${fmtTs(r.ts)}</td><td>${escape(r.method)}</td><td>${escape(r.error)}</td></tr>`).join('')
        : `<tr><td colspan="3" class="muted">No errors in window.</td></tr>`;
    return `<!doctype html><html><head><meta charset="utf-8"/><style>
body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); padding: 12px; }
h2 { margin-top: 0; }
.metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-bottom: 16px; }
.card { background: var(--vscode-editor-inactiveSelectionBackground); padding: 10px; border-radius: 4px; }
.card .v { font-size: 22px; font-weight: 600; }
.card .l { font-size: 11px; opacity: 0.7; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 4px 8px; text-align: left; border-bottom: 1px solid var(--vscode-panel-border); font-size: 12px; }
.muted { opacity: 0.6; font-style: italic; }
button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; padding: 4px 10px; margin-right: 8px; cursor: pointer; }
</style></head><body>
<h2>Foundry Telemetry — last ${s.window_hours}h</h2>
<div class="metrics">
  <div class="card"><div class="v">${s.calls}</div><div class="l">calls</div></div>
  <div class="card"><div class="v">${s.prompt_tokens}</div><div class="l">prompt tokens</div></div>
  <div class="card"><div class="v">${s.completion_tokens}</div><div class="l">completion tokens</div></div>
  <div class="card"><div class="v">${s.errors}</div><div class="l">errors</div></div>
</div>
<p><button id="r">Refresh</button><button id="c">Clear telemetry</button></p>
<h3>Recent errors</h3>
<table><thead><tr><th>Time</th><th>Method</th><th>Error</th></tr></thead><tbody>${tableRows}</tbody></table>
<script>
const v = acquireVsCodeApi();
document.getElementById('r').addEventListener('click', () => v.postMessage({ type: 'refresh' }));
document.getElementById('c').addEventListener('click', () => v.postMessage({ type: 'clear' }));
</script>
</body></html>`;
}

function renderError(msg: string): string {
    return `<!doctype html><html><body style="font-family: var(--vscode-font-family); padding: 12px;">
<h2>Foundry Telemetry</h2>
<p style="color: var(--vscode-errorForeground);">Failed to fetch: ${escape(msg)}</p>
</body></html>`;
}

function fmtTs(ts: number): string {
    try { return new Date(ts * 1000).toLocaleString(); } catch { return String(ts); }
}
function escape(s: string): string {
    return s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c] ?? c));
}
