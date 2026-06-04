# Security Contract

## The Hard Lock

`foundry-copilot` is built around one non-negotiable promise: **every byte of model traffic goes to a Microsoft Foundry endpoint, authenticated by Microsoft Entra ID.**

This promise is enforced by three independent layers. Bypassing any one of them is a security bug — please report via the process below.

### Layer 1 — Endpoint allow-list (Go)

File: [`core/internal/foundry/lock.go`](core/internal/foundry/lock.go)

A compiled-in list of host suffixes is the single source of truth:

```go
var AllowedHostSuffixes = []string{
    ".services.ai.azure.com",
    ".cognitiveservices.azure.com",
    ".openai.azure.com",
    ".inference.ml.azure.com",
}
```

An `http.RoundTripper` wrapper (`lockedTransport`) re-validates the request URL on **every** outbound HTTP call. Mismatch returns a typed `ErrEndpointNotFoundry` error and never touches the network.

Suffix matching is **dot-anchored** — `example.com.services.ai.azure.com.attacker.com` is rejected; only true subdomains of an allowed host pass.

### Layer 2 — Auth issuer pinning (Go)

File: [`core/internal/foundry/auth.go`](core/internal/foundry/auth.go)

The binary contains **no API-key code path**. Only `azidentity.DefaultAzureCredential` (which itself only emits Entra ID tokens) is wired into the Foundry client. The JWT `iss` claim is checked pre-flight against the Microsoft STS endpoints.

### Layer 3 — Settings UI regex (TS)

File: `extension/package.json` → `contributes.configuration.properties.foundryCopilot.endpoint.pattern`

The same allow-list is encoded as a JSON-schema regex. VS Code rejects invalid URLs at the settings UI layer **before** they reach the sidecar.

## Non-bypasses (by design)

- There is **no** `foundryCopilot.disableLock` setting.
- There is **no** `FOUNDRY_COPILOT_ALLOW_INSECURE` environment variable.
- There is **no** dynamic registration of additional host suffixes.
- Tests that attempt to inject non-Foundry hosts via `os.Setenv` or test-only build tags **fail CI**.

## Reporting a vulnerability

If you discover a way to make the extension issue model requests to a non-Foundry host:

1. Do **not** open a public issue.
2. Email `patmeh1@users.noreply.github.com` (or open a private Security Advisory on GitHub).
3. Include reproduction steps and the commit hash.

## Threat model (out of scope)

- Attackers with write access to `core/internal/foundry/lock.go` — they can change the allow-list. Mitigation: code review + branch protection on `main`.
- Attackers who run a separate process and exfiltrate from the same machine — outside this project's control.
- Attackers who MITM the user's machine — Foundry endpoints are HTTPS only; transport security is delegated to the Go stdlib + system trust store.
