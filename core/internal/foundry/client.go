package foundry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

// AzureAPIVersion is the api-version query parameter we append to every
// rewritten Azure OpenAI request. 2024-10-21 is the latest GA release at
// the time of writing and supports the chat/completions, embeddings and
// completions surfaces we use.
const AzureAPIVersion = "2024-10-21"

// Client is the thin Foundry wrapper used by the rest of the sidecar.
// All outbound traffic goes through a LockedTransport (see lock.go) AND
// every request is decorated with an Entra ID bearer token via middleware.
type Client struct {
	endpoint string
	inner    *openai.Client
	cred     azcore.TokenCredential
}

// NewClient constructs a Foundry client. It validates the endpoint against
// the hard-lock allow-list, wires a LockedTransport into the HTTP client
// so subsequent requests are double-checked, and installs an Entra-bearer
// middleware that fetches/refreshes a token from the supplied credential.
func NewClient(endpoint string, cred azcore.TokenCredential) (*Client, error) {
	if err := ValidateEndpoint(endpoint); err != nil {
		return nil, err
	}
	if cred == nil {
		return nil, fmt.Errorf("foundry-copilot: nil credential")
	}
	httpClient := LockedHTTPClient()
	bearer := newBearerMiddleware(cred)
	azureRoute := newAzureRouteMiddleware(AzureAPIVersion)

	inner := openai.NewClient(
		option.WithBaseURL(endpoint),
		option.WithHTTPClient(httpClient),
		// Azure routing MUST run before the bearer middleware so the URL
		// is the final Azure-style path by the time the request is signed
		// and dispatched through LockedTransport.
		option.WithMiddleware(azureRoute),
		option.WithMiddleware(bearer.middleware),
		// Prevent the SDK from sending an OpenAI API key (we are Entra-only).
		option.WithAPIKey(""),
	)
	return &Client{endpoint: endpoint, inner: &inner, cred: cred}, nil
}

// Endpoint returns the validated endpoint URL.
func (c *Client) Endpoint() string { return c.endpoint }

// Inner exposes the underlying openai.Client. The hard lock still applies
// because the http.Client was injected at construction.
func (c *Client) Inner() *openai.Client { return c.inner }

// ChatRequest is the sidecar's normalised chat shape — kept stable across
// SDK revisions so the RPC surface doesn't churn.
type ChatRequest struct {
	Deployment  string        `json:"deployment"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float32      `json:"temperature,omitempty"`
	MaxTokens   *int32        `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

// ChatMessage is a single turn in a conversation.
type ChatMessage struct {
	Role       string         `json:"role"` // "system" | "user" | "assistant" | "tool"
	Content    string         `json:"content"`
	Name       string         `json:"name,omitempty"`         // for tool messages
	ToolID     string         `json:"tool_id,omitempty"`      // deprecated alias for ToolCallID
	ToolCallID string         `json:"tool_call_id,omitempty"` // for role=="tool"
	ToolCalls  []ToolCallSpec `json:"tool_calls,omitempty"`   // for role=="assistant"
}

// ToolCallSpec is the assistant's request to invoke a tool. Mirrors the
// openai-go function tool-call shape.
type ToolCallSpec struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string from the model
}

// ChatChunk is one streamed delta sent to the extension.
type ChatChunk struct {
	Delta        string `json:"delta,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
	Err          string `json:"error,omitempty"`
}

// Chat streams the response from Foundry. The callback is invoked once per
// chunk; it must be cheap (the network read blocks on it). The hard-lock
// contract is in place because c.inner uses LockedTransport + bearer
// middleware.
func (c *Client) Chat(ctx context.Context, req ChatRequest, onChunk func(ChatChunk) error) error {
	if c.inner == nil {
		return fmt.Errorf("foundry-copilot: client not initialised")
	}
	if req.Deployment == "" {
		return fmt.Errorf("foundry-copilot: empty deployment name")
	}
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(req.Deployment),
		Messages: toOpenAIMessages(req.Messages),
	}
	if req.Temperature != nil {
		params.Temperature = param.NewOpt(float64(*req.Temperature))
	}
	if req.MaxTokens != nil {
		params.MaxCompletionTokens = param.NewOpt(int64(*req.MaxTokens))
	}
	stream := c.inner.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()
	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		out := ChatChunk{
			Delta:        choice.Delta.Content,
			FinishReason: string(choice.FinishReason),
		}
		if err := onChunk(out); err != nil {
			return err
		}
	}
	if err := stream.Err(); err != nil && !errors.Is(err, io.EOF) {
		_ = onChunk(ChatChunk{Err: err.Error()})
		return err
	}
	return nil
}

// toOpenAIMessages converts our normalised ChatMessage slice into the
// param-union slice openai-go expects. Handles tool messages and
// assistant turns that carry tool_calls.
func toOpenAIMessages(msgs []ChatMessage) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "tool":
			id := m.ToolCallID
			if id == "" {
				id = m.ToolID
			}
			out = append(out, openai.ToolMessage(m.Content, id))
		case "assistant":
			if len(m.ToolCalls) == 0 {
				out = append(out, openai.AssistantMessage(m.Content))
				continue
			}
			calls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					},
				})
			}
			ap := openai.ChatCompletionAssistantMessageParam{
				ToolCalls: calls,
			}
			if m.Content != "" {
				ap.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: param.NewOpt(m.Content),
				}
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &ap})
		case "user":
			fallthrough
		default:
			out = append(out, openai.UserMessage(m.Content))
		}
	}
	return out
}

// Embed returns one embedding vector per input string. Inputs are sent
// in a single request; callers should batch (≤ ~64 inputs per call) to
// stay under the embedding endpoint's token budget. The hard-lock
// HTTP client + bearer middleware apply just like Chat().
func (c *Client) Embed(ctx context.Context, deployment string, inputs []string) ([][]float32, error) {
	if c.inner == nil {
		return nil, fmt.Errorf("foundry-copilot: client not initialised")
	}
	if deployment == "" {
		return nil, fmt.Errorf("foundry-copilot: empty embedding deployment")
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(deployment),
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: inputs},
	}
	resp, err := c.inner.Embeddings.New(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(inputs))
	for _, e := range resp.Data {
		idx := int(e.Index)
		if idx < 0 || idx >= len(inputs) {
			continue
		}
		v := make([]float32, len(e.Embedding))
		for i, x := range e.Embedding {
			v[i] = float32(x)
		}
		out[idx] = v
	}
	return out, nil
}

// Complete runs a single non-streaming chat completion and returns the
// assistant's text. Used by Phase 4 inline completions; the model is
// instructed to return only the missing middle between prefix and suffix.
func (c *Client) Complete(ctx context.Context, deployment string, messages []ChatMessage, maxTokens int32) (string, error) {
	if c.inner == nil {
		return "", fmt.Errorf("foundry-copilot: client not initialised")
	}
	if deployment == "" {
		return "", fmt.Errorf("foundry-copilot: empty deployment name")
	}
	params := openai.ChatCompletionNewParams{
		Model:               shared.ChatModel(deployment),
		Messages:            toOpenAIMessages(messages),
		Temperature:         param.NewOpt(0.1),
		MaxCompletionTokens: param.NewOpt(int64(maxTokens)),
	}
	resp, err := c.inner.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Message.Content, nil
}

// ─── Entra bearer middleware ───────────────────────────────────────────────
//
// openai-go's middleware sees an http.Request just before send. We use it
// to set the Authorization header with a fresh Entra token. Tokens are
// cached and refreshed ~5 minutes before expiry.

type bearerMiddleware struct {
	cred azcore.TokenCredential
	mu   sync.Mutex
	tok  azcore.AccessToken
}

func newBearerMiddleware(cred azcore.TokenCredential) *bearerMiddleware {
	return &bearerMiddleware{cred: cred}
}

func (b *bearerMiddleware) token(ctx context.Context) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tok.Token != "" && time.Until(b.tok.ExpiresOn) > 5*time.Minute {
		return b.tok.Token, nil
	}
	t, err := b.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{FoundryScope}})
	if err != nil {
		return "", err
	}
	b.tok = t
	return t.Token, nil
}

func (b *bearerMiddleware) middleware(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	tok, err := b.token(req.Context())
	if err != nil {
		return nil, fmt.Errorf("foundry-copilot: acquire bearer: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return next(req)
}

// ─── Azure URL routing middleware ──────────────────────────────────────────
//
// openai-go was designed for api.openai.com and constructs URLs like
// "{baseURL}/chat/completions". Azure OpenAI / Microsoft Foundry expects
// "{baseURL}/openai/deployments/{deployment}/chat/completions?api-version=..."
// with the deployment name in the URL path and an api-version query param.
//
// This middleware bridges that mismatch: it reads the JSON body, lifts the
// "model" field (which the sidecar populates with the deployment name),
// rewrites the request URL to Azure's pattern, and adds the api-version
// query parameter. The original body bytes are preserved verbatim — Azure
// also accepts (and ignores) the "model" field in the body.
//
// Paths handled: /chat/completions, /completions, /embeddings. Anything
// else is passed through unchanged so the lock + bearer still get a chance
// to refuse.

func newAzureRouteMiddleware(apiVersion string) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	return func(req *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if req == nil || req.URL == nil {
			return next(req)
		}
		path := req.URL.Path
		if !isOpenAIRoutablePath(path) {
			return next(req)
		}
		body, err := readAndRestoreBody(req)
		if err != nil {
			return nil, fmt.Errorf("foundry-copilot: azure-route: read body: %w", err)
		}
		deployment := extractModelField(body)
		if deployment == "" {
			// No model in body — can't route to a deployment. Surface a
			// clear error rather than letting Azure respond with a generic
			// 404 that wastes a token on each chat turn.
			return nil, fmt.Errorf("foundry-copilot: azure-route: request body missing \"model\" field for path %q", path)
		}
		newPath := azureRoutePath(path, deployment)
		req.URL.Path = newPath
		q := req.URL.Query()
		if q.Get("api-version") == "" {
			q.Set("api-version", apiVersion)
			req.URL.RawQuery = q.Encode()
		}
		return next(req)
	}
}

// isOpenAIRoutablePath reports whether the request targets a known OpenAI
// surface that Azure exposes under /openai/deployments/{name}/...
func isOpenAIRoutablePath(p string) bool {
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		return true
	case strings.HasSuffix(p, "/completions") && !strings.HasSuffix(p, "/chat/completions"):
		return true
	case strings.HasSuffix(p, "/embeddings"):
		return true
	}
	return false
}

// azureRoutePath converts an OpenAI-style path to the Azure-style path with
// the deployment name embedded after "/openai/deployments/".
func azureRoutePath(p, deployment string) string {
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		return "/openai/deployments/" + deployment + "/chat/completions"
	case strings.HasSuffix(p, "/embeddings"):
		return "/openai/deployments/" + deployment + "/embeddings"
	case strings.HasSuffix(p, "/completions"):
		return "/openai/deployments/" + deployment + "/completions"
	}
	return p
}

// readAndRestoreBody drains req.Body, restores it (and GetBody) so the
// downstream RoundTrip can re-read it, and returns the raw bytes.
func readAndRestoreBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.ContentLength = int64(len(body))
	return body, nil
}

// extractModelField returns the value of the top-level "model" field in a
// JSON body, or "" if absent / unparseable.
func extractModelField(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	return probe.Model
}
