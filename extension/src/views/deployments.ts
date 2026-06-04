// deployments.ts — tree view of Foundry deployments. Calls deploy/list on
// the sidecar (which itself either returns ARM data or synthesises a one-
// entry view from the configured chat deployment) and renders each entry
// with its capabilities + state. Refresh via a command in the title bar.
import * as vscode from 'vscode';
import { Deployment, Methods, RpcClient } from '../sidecar/rpc';

class DeploymentItem extends vscode.TreeItem {
    constructor(public readonly dep: Deployment) {
        super(dep.name, vscode.TreeItemCollapsibleState.None);
        this.description = `${dep.model}${dep.sku ? ` · ${dep.sku}` : ''}${dep.state ? ` · ${dep.state}` : ''}`;
        this.tooltip = JSON.stringify(dep, null, 2);
        this.iconPath = new vscode.ThemeIcon(dep.state === 'Succeeded' ? 'cloud' : 'cloud-upload');
        this.contextValue = 'foundry.deployment';
    }
}

class EmptyItem extends vscode.TreeItem {
    constructor(label: string, desc: string) {
        super(label, vscode.TreeItemCollapsibleState.None);
        this.description = desc;
        this.iconPath = new vscode.ThemeIcon('info');
    }
}

class DeploymentsProvider implements vscode.TreeDataProvider<vscode.TreeItem> {
    private readonly emitter = new vscode.EventEmitter<vscode.TreeItem | undefined>();
    readonly onDidChangeTreeData = this.emitter.event;
    constructor(private readonly rpc: RpcClient) {}
    refresh(): void { this.emitter.fire(undefined); }
    getTreeItem(item: vscode.TreeItem): vscode.TreeItem { return item; }
    async getChildren(): Promise<vscode.TreeItem[]> {
        try {
            const list = await Methods.deployList(this.rpc, {});
            if (!list?.length) return [new EmptyItem('No deployments', 'configure endpoint + subscription_id/resource_group/account_name')];
            return list.map((d) => new DeploymentItem(d));
        } catch (err) {
            return [new EmptyItem('deploy/list failed', (err as Error).message)];
        }
    }
}

export function registerDeploymentsView(context: vscode.ExtensionContext, rpc: RpcClient): void {
    const provider = new DeploymentsProvider(rpc);
    context.subscriptions.push(
        vscode.window.registerTreeDataProvider('foundryCopilot.deployments', provider),
        vscode.commands.registerCommand('foundryCopilot.deployments.open', () =>
            vscode.commands.executeCommand('foundryCopilot.deployments.focus'),
        ),
        vscode.commands.registerCommand('foundryCopilot.deployments.refresh', () => provider.refresh()),
    );
}
