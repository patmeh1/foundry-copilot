// Package foundry contains the Microsoft Foundry client AND the hard lock that
// guarantees every outbound request stays on a Foundry endpoint.
//
// The lock is the single most security-critical component of this project.
// Read SECURITY.md before changing anything in this file.
package foundry

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// AllowedHostSuffixes is the dot-anchored list of host suffixes considered
// "Microsoft Foundry endpoints". A request URL whose hostname is exactly one
// of these suffixes (without leading dot) OR ends in ".<suffix>" is allowed.
//
// This list MUST stay short and well-justified. Adding a suffix is a security
// review event — see SECURITY.md.
var AllowedHostSuffixes = []string{
	".services.ai.azure.com",
	".cognitiveservices.azure.com",
	".openai.azure.com",
	".inference.ml.azure.com",
}

// ErrEndpointNotFoundry is returned when the lock rejects a URL.
var ErrEndpointNotFoundry = errors.New("foundry-copilot: endpoint is not a Microsoft Foundry host (hard lock)")

// ValidateEndpoint returns nil if rawURL points at an allowed Foundry host.
// It rejects:
//   - non-https schemes (Foundry is HTTPS only)
//   - IP literals (no IP-based Foundry endpoints exist)
//   - hostnames that merely *contain* an allowed suffix as a substring
//     (e.g. "example.com.services.ai.azure.com.attacker.com")
//   - empty / unparseable URLs
//
// Suffix matching is dot-anchored: "myproj.services.ai.azure.com" passes,
// "services.ai.azure.com" passes (it equals the base), but
// "evilservices.ai.azure.com" is rejected because there is no dot boundary.
func ValidateEndpoint(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: empty URL", ErrEndpointNotFoundry)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrEndpointNotFoundry, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: scheme %q is not https", ErrEndpointNotFoundry, u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("%w: missing host", ErrEndpointNotFoundry)
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("%w: IP literal %q not allowed", ErrEndpointNotFoundry, host)
	}
	for _, suffix := range AllowedHostSuffixes {
		base := strings.TrimPrefix(suffix, ".")
		// exact-match base host (e.g. "services.ai.azure.com") OR true subdomain.
		if host == base || strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return fmt.Errorf("%w: host %q", ErrEndpointNotFoundry, host)
}

// LockedTransport is an http.RoundTripper that re-validates every request URL
// against the Foundry allow-list before forwarding to the wrapped transport.
// It exists to defend against any code path that might construct an
// http.Request with a different host AFTER configuration has been validated.
type LockedTransport struct {
	Inner http.RoundTripper
}

// NewLockedTransport wraps inner. If inner is nil, http.DefaultTransport is used.
func NewLockedTransport(inner http.RoundTripper) *LockedTransport {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &LockedTransport{Inner: inner}
}

// RoundTrip implements http.RoundTripper.
func (t *LockedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("%w: nil request", ErrEndpointNotFoundry)
	}
	if err := ValidateEndpoint(req.URL.String()); err != nil {
		return nil, err
	}
	return t.Inner.RoundTrip(req)
}

// LockedHTTPClient returns an *http.Client whose Transport is the locked
// wrapper. Always use this client (or one whose transport is wrapped) for
// outbound model traffic. The azopenai SDK client should be constructed with
// this http.Client injected.
func LockedHTTPClient() *http.Client {
	return &http.Client{Transport: NewLockedTransport(nil)}
}
