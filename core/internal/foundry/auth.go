package foundry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// ErrUntrustedIssuer is returned when an acquired Entra ID token has an
// issuer that is not a recognised Microsoft STS endpoint.
var ErrUntrustedIssuer = errors.New("foundry-copilot: token issuer is not a trusted Microsoft Entra STS")

// FoundryScope is the OAuth scope used for Microsoft Cognitive Services /
// Foundry data-plane access. See
// https://learn.microsoft.com/azure/ai-services/authentication?tabs=token
const FoundryScope = "https://cognitiveservices.azure.com/.default"

// trustedIssuerPrefixes is the dot/slash-anchored list of acceptable JWT
// `iss` claim prefixes for Microsoft Entra ID tokens.
var trustedIssuerPrefixes = []string{
	"https://sts.windows.net/",
	"https://login.microsoftonline.com/",
}

// NewCredential returns a DefaultAzureCredential. No API-key path is exposed
// from this package — by design, the only way to authenticate is Entra ID.
func NewCredential() (azcore.TokenCredential, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("foundry-copilot: build credential: %w", err)
	}
	return cred, nil
}

// VerifyIssuer acquires a token for the Foundry scope and checks its `iss`
// claim against trustedIssuerPrefixes. Call this once at startup to fail
// fast if the wrong credential ends up wired in.
func VerifyIssuer(ctx context.Context, cred azcore.TokenCredential) error {
	tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{FoundryScope}})
	if err != nil {
		return fmt.Errorf("foundry-copilot: acquire token: %w", err)
	}
	iss, err := jwtIssuer(tok.Token)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUntrustedIssuer, err)
	}
	for _, p := range trustedIssuerPrefixes {
		if strings.HasPrefix(iss, p) {
			return nil
		}
	}
	return fmt.Errorf("%w: iss=%q", ErrUntrustedIssuer, iss)
}

// jwtIssuer extracts the `iss` claim from a JWT *without* verifying the
// signature. The signature is already verified by Microsoft's STS — we just
// re-read iss to make sure the credential we got is from a Microsoft STS.
func jwtIssuer(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("not a JWT (expected 3 segments, got %d)", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some tokens use std base64 with padding stripped — retry tolerantly.
		payload, err = base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("decode payload: %w", err)
		}
	}
	var claims struct {
		Iss string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse claims: %w", err)
	}
	if claims.Iss == "" {
		return "", errors.New("missing iss claim")
	}
	return claims.Iss, nil
}
