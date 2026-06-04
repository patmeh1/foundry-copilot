// provider.ts registers a LanguageModelChatProvider so Foundry deployments
// show up in the standard "Select Model" picker that VS Code uses for
// chat-extensible features (when those features call vscode.lm.* APIs).
//
// v0.2.0 scaffold: registers the provider only if the proposed API exists
// at runtime. The full streaming wiring (delegating to chat/start via
// chunk notifications) lands in v0.2.1. Until then the provider returns a
// canned message so users see Foundry models in the picker UI even before
// the data path is wired.
import * as vscode from 'vscode';
import { RpcClient } from '../sidecar/rpc';

interface RegisterableLm {
    registerChatModelProvider?: (id: string, provider: unknown, metadata: unknown) => vscode.Disposable;
}

export function registerLmProvider(
    context: vscode.ExtensionContext,
    rpc: RpcClient,
    output: vscode.OutputChannel,
): vscode.Disposable | undefined {
    const enabled = vscode.workspace
        .getConfiguration('foundryCopilot')
        .get<boolean>('lmProvider.enabled', true);
    if (!enabled) {
        output.appendLine('[lm] LanguageModelChatProvider disabled by foundryCopilot.lmProvider.enabled');
        return undefined;
    }

    const deployment = vscode.workspace
        .getConfiguration('foundryCopilot')
        .get<string>('chatDeployment', '');
    if (!deployment) {
        output.appendLine('[lm] foundryCopilot.chatDeployment not set; LM provider not registered');
        return undefined;
    }

    const lm = vscode.lm as unknown as RegisterableLm;
    if (typeof lm.registerChatModelProvider !== 'function') {
        output.appendLine(
            '[lm] vscode.lm.registerChatModelProvider not available in this VS Code build (proposed API); skipping',
        );
        return undefined;
    }

    output.appendLine(`[lm] registering Foundry LanguageModelChatProvider for deployment "${deployment}"`);

    // Scaffold provider: returns a single canned chunk so users see it in
    // the UI. Real streaming wiring lands in v0.2.1.
    const provider = {
        provideLanguageModelChatResponse: async (
            _messages: unknown[],
            _options: unknown,
            _progress: { report: (v: unknown) => void },
            _token: vscode.CancellationToken,
        ) => {
            void rpc;
            const msg = `[Foundry LM provider — v0.2 scaffold] Streaming integration lands in v0.2.1. Use the @foundry chat participant or the Foundry Chat view for full functionality today.`;
            _progress.report({ index: 0, part: msg });
        },
        provideTokenCount: async (text: string) => Math.ceil(text.length / 4),
    };

    const metadata = {
        vendor: 'foundry',
        name: 'Foundry Chat',
        family: 'chat',
        version: '0.2.0',
        maxInputTokens: 128_000,
        maxOutputTokens: 16_000,
    };

    const disposable = lm.registerChatModelProvider!(deployment, provider, metadata);
    context.subscriptions.push(disposable);
    return disposable;
}
