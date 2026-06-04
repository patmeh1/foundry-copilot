// deployments.ts — tree view of Foundry deployments. v0.2 scaffold shows a
// single "no data" placeholder. v0.2.1 wires deploy/list against the sidecar.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

class Placeholder extends vscode.TreeItem {
    constructor(label: string, desc: string) {
        super(label, vscode.TreeItemCollapsibleState.None);
        this.description = desc;
        this.iconPath = new vscode.ThemeIcon('cloud');
    }
}

class DeploymentsProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
    constructor(private readonly rpc: RpcClient) {}
    getTreeItem(item: vscode.TreeItem): vscode.TreeItem { return item; }
    async getChildren(): Promise<vscode.TreeItem[]> {
        void this.rpc;
        return [new Placeholder('No deployments listed yet', 'v0.2 scaffold — wires up in v0.2.1')];
    }
}

export function registerDeploymentsView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    const provider = new DeploymentsProvider(rpc);
    context.subscriptions.push(
        vscode.window.registerTreeDataProvider('foundryCopilot.deployments', provider),
        vscode.commands.registerCommand('foundryCopilot.deployments.open', () =>
            vscode.commands.executeCommand('foundryCopilot.deployments.focus'),
        ),
    );
}
