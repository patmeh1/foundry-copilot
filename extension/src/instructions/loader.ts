// instructions/loader.ts loads workspace-level instructions from
// .foundry/instructions/*.md and offers a one-time migration from any
// existing .github/copilot-instructions.md so teams don't have to maintain
// two files.
import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs/promises';

const DISMISS_KEY = 'foundryCopilot.instructions.migratePromptShown';

export async function loadFoundryInstructions(workspaceRoot: string): Promise<string[]> {
    const dir = path.join(workspaceRoot, '.foundry', 'instructions');
    try {
        const entries = await fs.readdir(dir);
        const md = entries.filter((f) => f.toLowerCase().endsWith('.md')).sort();
        const out: string[] = [];
        for (const f of md) {
            out.push(await fs.readFile(path.join(dir, f), 'utf8'));
        }
        return out;
    } catch (err) {
        if ((err as NodeJS.ErrnoException).code === 'ENOENT') return [];
        throw err;
    }
}

export async function maybePromptCopilotMigration(
    context: vscode.ExtensionContext,
    workspaceRoot: string,
    output: vscode.OutputChannel,
): Promise<void> {
    if (context.globalState.get<boolean>(DISMISS_KEY, false)) return;
    const copilotInstr = path.join(workspaceRoot, '.github', 'copilot-instructions.md');
    const foundryInstr = path.join(workspaceRoot, '.foundry', 'instructions');
    let hasCopilot = false;
    try {
        await fs.access(copilotInstr);
        hasCopilot = true;
    } catch {
        return;
    }
    let hasFoundry = false;
    try {
        await fs.access(foundryInstr);
        hasFoundry = true;
    } catch {
        /* not yet */
    }
    if (!hasCopilot || hasFoundry) return;

    const choice = await vscode.window.showInformationMessage(
        'Foundry Copilot found .github/copilot-instructions.md. Use it as the starting point for .foundry/instructions/?',
        'Copy',
        'Open both',
        "Don't ask again",
    );
    if (choice === 'Copy') {
        const data = await fs.readFile(copilotInstr, 'utf8');
        await fs.mkdir(foundryInstr, { recursive: true });
        await fs.writeFile(path.join(foundryInstr, '00-migrated-from-copilot.md'), data, 'utf8');
        output.appendLine('[instructions] copied copilot-instructions.md → .foundry/instructions/');
        void context.globalState.update(DISMISS_KEY, true);
    } else if (choice === 'Open both') {
        await vscode.commands.executeCommand('vscode.open', vscode.Uri.file(copilotInstr));
    } else if (choice === "Don't ask again") {
        void context.globalState.update(DISMISS_KEY, true);
    }
}
