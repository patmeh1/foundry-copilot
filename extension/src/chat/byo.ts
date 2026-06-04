// byo.ts — "Bring-Your-Own Chat" controller. Enforces the core tenant
// that the Foundry chat surface IS the chat surface. Specifically:
//
//   1. Foundry Copilot does NOT register a `vscode.chat.createChatParticipant`
//      — that would route prompts through VS Code's built-in chat panel,
//      which is provided by GitHub Copilot Chat (no BAA). Refusing to
//      participate is what makes the built-in chat command-centre disappear
//      when the BAA controller disables GitHub.copilot-chat.
//
//   2. We also defensively turn OFF the `chat.commandCenter.enabled` setting
//      (workspace scope) so that if another extension ships a chat
//      participant later, the title-bar chat input still doesn't appear.
//      Opt out via `foundryCopilot.byoChat.hideBuiltInChat = false`.
//
//   3. On activation we focus the Foundry chat view so users see Foundry's
//      surface first. Opt out via `foundryCopilot.byoChat.focusOnActivation`.
//
//   4. A keybinding (Cmd+Alt+I / Ctrl+Alt+I) jumps to the Foundry chat
//      regardless of current focus.
//
// This module is intentionally narrow: it touches user settings only when
// the user has opted in (the default), and only at workspace scope so it
// never bleeds into their global VS Code state.
import * as vscode from 'vscode';

const NS = 'foundryCopilot.byoChat';
const COMMAND_FOCUS = 'foundryCopilot.chat.focus';
const COMMAND_RECHECK = 'foundryCopilot.byoChat.recheck';
const CHAT_VIEW_ID = 'foundryCopilot.chatView';

interface ByoConfig {
    hideBuiltInChat: boolean;
    focusOnActivation: boolean;
}

function readConfig(): ByoConfig {
    const c = vscode.workspace.getConfiguration(NS);
    return {
        hideBuiltInChat: c.get<boolean>('hideBuiltInChat', true),
        focusOnActivation: c.get<boolean>('focusOnActivation', true),
    };
}

/**
 * Enforce the BYO-chat tenant. Idempotent and safe to call on activation
 * and on every configuration change.
 */
export async function enforceByoChat(output: vscode.OutputChannel): Promise<void> {
    const cfg = readConfig();
    if (cfg.hideBuiltInChat) {
        const target = vscode.ConfigurationTarget.Workspace;
        const chatCfg = vscode.workspace.getConfiguration('chat');
        try {
            // chat.commandCenter.enabled controls the title-bar chat input.
            // If absent (older VS Code), update() throws — swallow.
            if (chatCfg.get<boolean>('commandCenter.enabled') !== false) {
                await chatCfg.update('commandCenter.enabled', false, target);
                output.appendLine('[byo-chat] disabled chat.commandCenter.enabled at workspace scope');
            }
        } catch (err) {
            output.appendLine(`[byo-chat] could not write chat.commandCenter.enabled: ${(err as Error).message}`);
        }
    }
}

export function registerByoChat(
    context: vscode.ExtensionContext,
    output: vscode.OutputChannel,
): void {
    // Foundry chat focus command + keybinding entry point.
    context.subscriptions.push(
        vscode.commands.registerCommand(COMMAND_FOCUS, async () => {
            // The activity-bar container id is `foundryCopilot`; the view id
            // is `foundryCopilot.chatView`. Focusing the view also reveals
            // the container.
            try {
                await vscode.commands.executeCommand(`${CHAT_VIEW_ID}.focus`);
            } catch {
                // Older VS Code: fall back to container focus.
                await vscode.commands.executeCommand('workbench.view.extension.foundryCopilot');
            }
        }),
        vscode.commands.registerCommand(COMMAND_RECHECK, () => enforceByoChat(output)),
        vscode.workspace.onDidChangeConfiguration((e) => {
            if (e.affectsConfiguration(NS) || e.affectsConfiguration('chat.commandCenter.enabled')) {
                void enforceByoChat(output);
            }
        }),
    );

    // Activation actions.
    void enforceByoChat(output).then(async () => {
        const cfg = readConfig();
        if (cfg.focusOnActivation) {
            try {
                await vscode.commands.executeCommand(COMMAND_FOCUS);
            } catch (err) {
                output.appendLine(`[byo-chat] focus-on-activation failed: ${(err as Error).message}`);
            }
        }
    });
}
