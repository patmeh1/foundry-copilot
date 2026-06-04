package foundry

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateEndpoint is the load-bearing test of this project.
// Any contributor PR that breaks one of these table rows MUST be rejected.
func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantPass bool
	}{
		// ── Pass: real Foundry hostnames ──────────────────────────────────
		{"services.ai subdomain", "https://myproj.services.ai.azure.com/openai/deployments/gpt-4o/chat/completions?api-version=2024-10-21", true},
		{"cognitiveservices subdomain", "https://myresource.cognitiveservices.azure.com/openai/deployments/x/chat/completions", true},
		{"openai.azure.com", "https://myresource.openai.azure.com/openai/deployments/x/chat/completions", true},
		{"inference.ml subdomain", "https://my-endpoint.inference.ml.azure.com/score", true},
		{"deep subdomain", "https://a.b.c.services.ai.azure.com/foo", true},
		{"uppercase host normalised", "https://MyProj.Services.AI.Azure.Com/foo", true},

		// ── Reject: non-Foundry hosts ──────────────────────────────────
		{"openai.com", "https://api.openai.com/v1/chat/completions", false},
		{"anthropic", "https://api.anthropic.com/v1/messages", false},
		{"ollama localhost", "http://localhost:11434/api/chat", false},
		{"local loopback IP", "http://127.0.0.1:8080/foo", false},
		{"private IPv4", "https://10.0.0.5/foo", false},
		{"IPv6 literal", "https://[::1]/foo", false},
		{"http (non-tls)", "http://myproj.services.ai.azure.com/foo", false},
		{"empty URL", "", false},
		{"missing host", "https:///foo", false},
		{"plain google", "https://www.google.com/", false},
		{"github raw", "https://raw.githubusercontent.com/x/y/main/z", false},

		// ── Reject: suffix-substring attacks ───────────────────────────
		{"suffix-substring trailing attacker domain",
			"https://myproj.services.ai.azure.com.attacker.com/foo", false},
		{"suffix appears mid-host",
			"https://foo-services.ai.azure.com-attacker.example/bar", false},
		{"no dot boundary (evil prefix on base)",
			"https://evilservices.ai.azure.com/x", false},
		{"unicode homograph (cyrillic а)",
			"https://myproj.services.\u0430i.azure.com/x", false},
		{"trailing dot lookalike",
			"https://services.ai.azure.com.example.org/x", false},
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
			if !gotPass && !errors.Is(err, ErrEndpointNotFoundry) {
				t.Fatalf("rejection error must wrap ErrEndpointNotFoundry, got: %v", err)
			}
		})
	}
}

// TestLockedTransport_BlocksNonFoundry ensures the LockedTransport refuses
// to dispatch even when the inner transport would otherwise accept anything.
// It uses an httptest server so we can prove the request never reached it.
func TestLockedTransport_BlocksNonFoundry(t *testing.T) {
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
	if !errors.Is(err, ErrEndpointNotFoundry) {
		t.Fatalf("expected ErrEndpointNotFoundry, got %v", err)
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
	if err == nil || !errors.Is(err, ErrEndpointNotFoundry) {
		t.Fatalf("expected ErrEndpointNotFoundry for nil request, got %v", err)
	}
}

// TestAllowedHostSuffixes_Format makes sure every suffix is dot-anchored.
// A suffix without a leading dot would enable substring attacks.
func TestAllowedHostSuffixes_Format(t *testing.T) {
	for _, s := range AllowedHostSuffixes {
		if !strings.HasPrefix(s, ".") {
			t.Errorf("suffix %q must start with a dot (dot-anchored matching)", s)
		}
		if strings.ContainsAny(s, " \t\n") {
			t.Errorf("suffix %q contains whitespace", s)
		}
	}
}
