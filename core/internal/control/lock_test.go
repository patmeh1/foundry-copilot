package control

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateEndpoint is the load-bearing security test for the control plane.
// Any PR that breaks one of these table rows MUST be rejected.
func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantPass bool
	}{
		// ── Pass: real ARM hostnames ───────────────────────────────────────
		{"management.azure.com root", "https://management.azure.com/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.CognitiveServices/accounts/acct/deployments?api-version=2024-10-01", true},
		{"management.azure.com simple", "https://management.azure.com/foo", true},
		{"deep subdomain", "https://westus.management.azure.com/foo", true},
		{"uppercase host normalised", "https://Management.Azure.Com/x", true},

		// ── Reject: non-ARM hosts ─────────────────────────────────────────
		{"openai.com", "https://api.openai.com/v1/chat/completions", false},
		{"foundry data-plane is NOT ARM", "https://myproj.services.ai.azure.com/openai/deployments/x/chat/completions", false},
		{"anthropic", "https://api.anthropic.com/v1/messages", false},
		{"ollama localhost", "http://localhost:11434/api/chat", false},
		{"local loopback IP", "http://127.0.0.1:8080/foo", false},
		{"private IPv4", "https://10.0.0.5/foo", false},
		{"IPv6 literal", "https://[::1]/foo", false},
		{"http (non-tls)", "http://management.azure.com/foo", false},
		{"empty URL", "", false},
		{"missing host", "https:///foo", false},
		{"plain google", "https://www.google.com/", false},

		// ── Reject: suffix-substring attacks ──────────────────────────────
		{"suffix-substring trailing attacker domain", "https://management.azure.com.attacker.com/foo", false},
		{"suffix appears mid-host", "https://foo-management.azure.com-attacker.example/bar", false},
		{"no dot boundary (evil prefix on base)", "https://evilmanagement.azure.com/x", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpoint(tt.url)
			gotPass := err == nil
			if gotPass != tt.wantPass {
				t.Fatalf("ValidateEndpoint(%q) pass=%v err=%v ; want pass=%v",
					tt.url, gotPass, err, tt.wantPass)
			}
			if !gotPass && !errors.Is(err, ErrEndpointNotControlPlane) {
				t.Fatalf("rejection error must wrap ErrEndpointNotControlPlane, got: %v", err)
			}
		})
	}
}

// TestLockedTransport_BlocksNonControl ensures the LockedTransport refuses
// to dispatch even when the inner transport would otherwise accept anything.
func TestLockedTransport_BlocksNonControl(t *testing.T) {
	reachedInner := false
	innerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedInner = true
	}))
	t.Cleanup(innerSrv.Close)

	lockedClient := &http.Client{Transport: NewLockedTransport(http.DefaultTransport)}

	// httptest server URL is loopback — must be rejected.
	resp, err := lockedClient.Get(innerSrv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("expected lock rejection for %s; got nil err", innerSrv.URL)
	}
	if !errors.Is(err, ErrEndpointNotControlPlane) {
		t.Fatalf("expected ErrEndpointNotControlPlane, got %v", err)
	}
	if reachedInner {
		t.Fatalf("lock failed: request reached the inner test server")
	}
}

// TestLockedTransport_NilRequest defends against any code path that passes
// a nil request / URL to the transport directly.
func TestLockedTransport_NilRequest(t *testing.T) {
	tr := NewLockedTransport(nil)
	_, err := tr.RoundTrip(nil)
	if err == nil || !errors.Is(err, ErrEndpointNotControlPlane) {
		t.Fatalf("expected ErrEndpointNotControlPlane for nil request, got %v", err)
	}
}

// TestAllowedControlHosts_Format makes sure every suffix is dot-anchored.
// A suffix without a leading dot would enable substring attacks.
func TestAllowedControlHosts_Format(t *testing.T) {
	for _, s := range AllowedControlHosts {
		if !strings.HasPrefix(s, ".") {
			t.Errorf("suffix %q must start with a dot (dot-anchored matching)", s)
		}
		if strings.ContainsAny(s, " \t\n") {
			t.Errorf("suffix %q contains whitespace", s)
		}
	}
}
