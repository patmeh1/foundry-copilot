# Foundry Copilot

GitHub Copilot–style coding assistant for VS Code — **locked** to models
deployed in your Microsoft Foundry / Azure AI tenant.

👉 **Full step-by-step setup:** see the
[User Guide](https://github.com/patmeh1/foundry-copilot/blob/main/docs/USER_GUIDE.md).

## Why this exists

GitHub Copilot calls models hosted by GitHub. This extension calls models
**only** in your Microsoft Foundry tenant, authenticated with Entra ID via
`DefaultAzureCredential`. The lock is enforced at three layers (HTTP
allow-list, JWT issuer allow-list, settings regex) so a misconfiguration
can't accidentally route traffic to a non-Foundry endpoint.

## Features

- **`@foundry` chat participant** with five slash commands:
  `/explain`, `/fix`, `/tests`, `/doc`, `/agent`.
- **Inline completions** powered by your Foundry completion deployment.
- **Tool-using agent** with built-in `fs_read`, `code_search`,
  `fs_write` (gated), and `shell` (gated) tools.
- **Workspace RAG** — `Foundry Copilot: Refresh Workspace Index` embeds
  your code and surfaces a `rag_search` tool for the agent.
- **MCP support** — configure any number of stdio Model Context Protocol
  servers; their tools are auto-injected into the agent loop.

## Setup

1. Set `foundryCopilot.endpoint` in VS Code settings, e.g.
   `https://my-resource.services.ai.azure.com`. The setting's regex
   rejects anything outside the Foundry allow-list.
2. Set `foundryCopilot.chatDeployment` to your chat model deployment name.
3. Run `az login` (or another Azure credential method).
4. Open the Copilot Chat panel and `@foundry hello`.

## License

MIT.
