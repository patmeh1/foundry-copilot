// Package control enforces the Azure Resource Manager (ARM) control-plane
// boundary for foundry-copilot v0.2.
//
// This package is the symmetric twin of core/internal/foundry: where
// foundry.LockedTransport restricts data-plane traffic to Foundry endpoints
// only, this package restricts control-plane traffic to the ARM endpoint
// only. v0.2 surfaces (deployments tree-view, telemetry export, billing
// summaries) MUST route through control.LockedHTTPClient() so the BAA
// boundary holds at the control-plane just as it does at the data-plane.
//
// Allowed host: management.azure.com (and any subdomain). Everything else
// — including api.openai.com, anthropic.com, google APIs, localhost,
// IP literals — is rejected before the request leaves the process.
package control

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// AllowedControlHosts is the dot-anchored allow-list. Adding entries here
// is a security-sensitive change — every PR that touches this slice MUST
// be reviewed for BAA implications. See SECURITY.md.
var AllowedControlHosts = []string{
	".management.azure.com",
}

// ErrEndpointNotControlPlane is returned (wrapped) for every rejection. The
// extension surfaces this verbatim so users can debug their configuration.
var ErrEndpointNotControlPlane = errors.New("endpoint is not an allowed Azure control-plane (management.azure.com) host")

// ValidateEndpoint enforces three rules:
//  1. HTTPS only — never plaintext
//  2. Hostname is not an IP literal (no leakage to private ranges)
//  3. Hostname has a dot-anchored suffix in AllowedControlHosts
func ValidateEndpoint(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: empty URL", ErrEndpointNotControlPlane)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: parse: %v", ErrEndpointNotControlPlane, err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: scheme must be https, got %q", ErrEndpointNotControlPlane, u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("%w: missing host", ErrEndpointNotControlPlane)
	}
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("%w: host is an IP literal (%s)", ErrEndpointNotControlPlane, host)
	}
	// Dot-anchor: prepend a dot to the host so suffix matching cannot
	// match across a label boundary (".management.azure.com" must match
	// ".myproj.management.azure.com" but never "management.azure.com.evil.com").
	dotHost := "." + host
	for _, suf := range AllowedControlHosts {
		if strings.HasSuffix(dotHost, suf) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrEndpointNotControlPlane, host)
}

// LockedTransport wraps an inner RoundTripper with a per-request re-validation
// of the destination URL. Even if a caller bypasses configuration and crafts
// a raw http.Request, the transport will refuse to dispatch.
type LockedTransport struct {
	Inner http.RoundTripper
}

// NewLockedTransport returns a LockedTransport wrapping the given inner
// transport. nil falls back to http.DefaultTransport.
func NewLockedTransport(inner http.RoundTripper) *LockedTransport {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &LockedTransport{Inner: inner}
}

// RoundTrip implements http.RoundTripper.
func (t *LockedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("%w: nil request", ErrEndpointNotControlPlane)
	}
	if err := ValidateEndpoint(req.URL.String()); err != nil {
		return nil, err
	}
	return t.Inner.RoundTrip(req)
}

// LockedHTTPClient returns an *http.Client whose transport rejects every
// request that does not target the ARM control-plane.
func LockedHTTPClient() *http.Client {
	return &http.Client{Transport: NewLockedTransport(nil)}
}
