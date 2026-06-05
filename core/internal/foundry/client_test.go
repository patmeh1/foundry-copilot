package foundry

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestAzureRouteMiddleware verifies that the URL-rewriting middleware
// correctly translates openai-go's OpenAI-style request URLs into the
// Azure OpenAI / Microsoft Foundry shape:
//
//	/chat/completions
//	  → /openai/deployments/<model>/chat/completions?api-version=<ver>
//
// The middleware must also preserve the original request body verbatim
// (Azure ignores the body's "model" field) and leave non-routable paths
// untouched.
func TestAzureRouteMiddleware(t *testing.T) {
	const apiVersion = "2024-10-21"
	mw := newAzureRouteMiddleware(apiVersion)

	tests := []struct {
		name        string
		path        string
		body        string
		wantPath    string
		wantQueryAV string // empty == don't assert
		wantErr     bool
	}{
		{
			name:        "chat completions",
			path:        "/chat/completions",
			body:        `{"model":"gpt-5.4","messages":[{"role":"user","content":"hi"}]}`,
			wantPath:    "/openai/deployments/gpt-5.4/chat/completions",
			wantQueryAV: apiVersion,
		},
		{
			name:        "embeddings",
			path:        "/embeddings",
			body:        `{"model":"text-embedding-3-small","input":["a","b"]}`,
			wantPath:    "/openai/deployments/text-embedding-3-small/embeddings",
			wantQueryAV: apiVersion,
		},
		{
			name:        "legacy completions",
			path:        "/completions",
			body:        `{"model":"gpt-3.5-turbo-instruct","prompt":"hi"}`,
			wantPath:    "/openai/deployments/gpt-3.5-turbo-instruct/completions",
			wantQueryAV: apiVersion,
		},
		{
			name:     "unrelated path passes through",
			path:     "/models",
			body:     `{}`,
			wantPath: "/models",
		},
		{
			name:    "routable path missing model is an error",
			path:    "/chat/completions",
			body:    `{"messages":[{"role":"user","content":"hi"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedReq := &http.Request{}
			next := func(r *http.Request) (*http.Response, error) {
				*capturedReq = *r
				// Drain body to ensure it survives the middleware.
				if r.Body != nil {
					b, _ := io.ReadAll(r.Body)
					capturedReq.Body = io.NopCloser(bytes.NewReader(b))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			u, _ := url.Parse("https://myproj.openai.azure.com" + tt.path)
			req := &http.Request{
				Method: "POST",
				URL:    u,
				Body:   io.NopCloser(strings.NewReader(tt.body)),
				Header: http.Header{},
			}
			resp, err := mw(req, next)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q with body %q, got none", tt.path, tt.body)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("expected response, got nil")
			}
			if capturedReq.URL.Path != tt.wantPath {
				t.Errorf("path: got %q want %q", capturedReq.URL.Path, tt.wantPath)
			}
			if tt.wantQueryAV != "" {
				if got := capturedReq.URL.Query().Get("api-version"); got != tt.wantQueryAV {
					t.Errorf("api-version: got %q want %q", got, tt.wantQueryAV)
				}
			}
			// Body should survive verbatim
			if capturedReq.Body != nil {
				b, _ := io.ReadAll(capturedReq.Body)
				if string(b) != tt.body {
					t.Errorf("body mutated: got %q want %q", string(b), tt.body)
				}
			}
		})
	}
}

// TestExtractModelField covers the JSON body probe.
func TestExtractModelField(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{`{"model":"gpt-5.4"}`, "gpt-5.4"},
		{`{"model":"text-embedding-3-small","input":["a"]}`, "text-embedding-3-small"},
		{`{}`, ""},
		{``, ""},
		{`not json`, ""},
		{`{"messages":[]}`, ""},
	}
	for _, tt := range tests {
		got := extractModelField([]byte(tt.body))
		if got != tt.want {
			t.Errorf("extractModelField(%q): got %q want %q", tt.body, got, tt.want)
		}
	}
}

// TestIsOpenAIRoutablePath ensures we correctly classify which paths
// need rewriting and don't accidentally double-rewrite already-azure paths.
func TestIsOpenAIRoutablePath(t *testing.T) {
	cases := map[string]bool{
		"/chat/completions":                      true,
		"/v1/chat/completions":                   true,
		"/embeddings":                            true,
		"/completions":                           true,
		"/openai/deployments/x/chat/completions": true, // suffix still matches; that's fine, model→same path
		"/models":                                false,
		"/openai/models":                         false,
		"/foo/bar":                               false,
	}
	for p, want := range cases {
		if got := isOpenAIRoutablePath(p); got != want {
			t.Errorf("isOpenAIRoutablePath(%q): got %v want %v", p, got, want)
		}
	}
}
