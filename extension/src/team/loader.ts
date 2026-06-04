// team/loader.ts — read .foundry/policy.json + .foundry/tools/*.json and
// watch for changes. The actual policy enforcement happens server-side in
// core/internal/team. This loader is a TS-side mirror so the extension can
// surface policy violations inline (v0.2.1).
import * as vscode from 'vscode';
import * as path from 'path';

export interface TeamPaths {
    policy: string;
    toolsDir: string;
    keysDir: string;
}

export function loadTeamConfig(workspaceRoot: string): TeamPaths {
    return {
        policy: path.join(workspaceRoot, '.foundry', 'policy.json'),
        toolsDir: path.join(workspaceRoot, '.foundry', 'tools'),
        keysDir: path.join(workspaceRoot, '.foundry', 'keys'),
    };
}

export function registerTeamWatcher(
    context: vscode.ExtensionContext,
    output: vscode.OutputChannel,
    onReload: () => void,
): void {
    const folder = vscode.workspace.workspaceFolders?.[0];
    if (!folder) return;
    const pattern = new vscode.RelativePattern(folder, '.foundry/**/*');
    const watcher = vscode.workspace.createFileSystemWatcher(pattern);
    const handler = (uri: vscode.Uri) => {
        output.appendLine(`[team] .foundry change: ${vscode.workspace.asRelativePath(uri)}`);
        onReload();
    };
    watcher.onDidCreate(handler);
    watcher.onDidChange(handler);
    watcher.onDidDelete(handler);
    context.subscriptions.push(watcher);
}
