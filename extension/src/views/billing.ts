// billing.ts — opens a webview that shows estimated spend over the last
// 24 hours plus a daily-budget editor. Estimates come from telemetry-
// derived prompt+completion tokens × a flat gpt-4o-mini-style rate; real
// cost-mgmt integration lands in v0.2.1.
import * as vscode from 'vscode';
import { BillingSummary, Methods, RpcClient } from '../sidecar/rpc';

export function registerBillingView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    context.subscriptions.push(
        vscode.commands.registerCommand('foundryCopilot.billing.open', async () => {
            const panel = vscode.window.createWebviewPanel(
                'foundryCopilot.billing',
                'Foundry Billing',
                vscode.ViewColumn.Active,
                { enableScripts: true },
            );
            const refresh = async () => {
                try {
                    const [summary, budget] = await Promise.all([
                        Methods.billingSummary(rpc, { window_hours: 24 }),
                        Methods.billingBudgetGet(rpc),
                    ]);
                    panel.webview.html = renderHtml(summary, budget.budget_usd);
                } catch (err) {
                    panel.webview.html = renderError((err as Error).message);
                }
            };
            panel.webview.onDidReceiveMessage(async (m) => {
                if (m?.type === 'refresh') await refresh();
                if (m?.type === 'set') {
                    const v = parseFloat(String(m.value ?? ''));
                    if (!isFinite(v) || v < 0) {
                        vscode.window.showWarningMessage('Budget must be a non-negative number.');
                        return;
                    }
                    try {
                        await Methods.billingBudgetSet(rpc, { budget_usd: v });
                        await refresh();
                    } catch (err) {
                        vscode.window.showErrorMessage(`billing/budget_set failed: ${(err as Error).message}`);
                    }
                }
            });
            await refresh();
        }),
    );
}

function renderHtml(s: BillingSummary, budget: number): string {
    const pct = budget > 0 ? Math.min(100, (s.estimated_usd / budget) * 100) : 0;
    return `<!doctype html><html><head><meta charset="utf-8"/><style>
body { font-family: var(--vscode-font-family); color: var(--vscode-foreground); padding: 12px; }
.metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
.card { background: var(--vscode-editor-inactiveSelectionBackground); padding: 10px; border-radius: 4px; }
.card .v { font-size: 22px; font-weight: 600; }
.card .l { font-size: 11px; opacity: 0.7; }
.bar { height: 8px; background: var(--vscode-panel-border); border-radius: 4px; overflow: hidden; margin-top: 10px; }
.bar > div { height: 100%; background: ${pct > 80 ? 'var(--vscode-charts-red)' : 'var(--vscode-charts-blue)'}; width: ${pct.toFixed(1)}%; }
button, input { background: var(--vscode-input-background); color: var(--vscode-input-foreground); border: 1px solid var(--vscode-input-border); padding: 4px 8px; font-family: inherit; }
button { background: var(--vscode-button-background); color: var(--vscode-button-foreground); border: none; cursor: pointer; }
.note { font-size: 11px; opacity: 0.7; margin-top: 10px; }
</style></head><body>
<h2>Foundry Billing — last ${s.window_hours}h</h2>
<div class="metrics">
  <div class="card"><div class="v">$${s.estimated_usd.toFixed(4)}</div><div class="l">estimated spend</div></div>
  <div class="card"><div class="v">${s.calls}</div><div class="l">calls</div></div>
  <div class="card"><div class="v">${s.prompt_tokens + s.completion_tokens}</div><div class="l">tokens</div></div>
</div>
<h3>Daily budget</h3>
<p>Current budget: $${budget.toFixed(2)} ${budget > 0 ? `(${pct.toFixed(0)}% used)` : '(no budget set)'}</p>
<div class="bar"><div></div></div>
<p><input type="text" id="b" placeholder="e.g. 5.00" value="${budget.toFixed(2)}" style="width:120px"/> <button id="set">Set budget</button> <button id="r">Refresh</button></p>
<p class="note">${escape(s.note)}</p>
<script>
const v = acquireVsCodeApi();
document.getElementById('r').addEventListener('click', () => v.postMessage({ type: 'refresh' }));
document.getElementById('set').addEventListener('click', () => v.postMessage({ type: 'set', value: document.getElementById('b').value }));
</script>
</body></html>`;
}

function renderError(msg: string): string {
    return `<!doctype html><html><body style="font-family: var(--vscode-font-family); padding: 12px;">
<h2>Foundry Billing</h2>
<p style="color: var(--vscode-errorForeground);">Failed to fetch: ${escape(msg)}</p>
</body></html>`;
}

function escape(s: string): string {
    return s.replace(/[&<>]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c] ?? c));
}
