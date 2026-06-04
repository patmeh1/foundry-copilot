package foundry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/shared"
)

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

	inner := openai.NewClient(
		option.WithBaseURL(endpoint),
		option.WithHTTPClient(httpClient),
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

// Embed returns a single embedding vector for each input. Stub for Phase 7.
func (c *Client) Embed(ctx context.Context, deployment string, inputs []string) ([][]float32, error) {
	if c.inner == nil {
		return nil, fmt.Errorf("foundry-copilot: client not initialised")
	}
	if deployment == "" {
		return nil, fmt.Errorf("foundry-copilot: empty embedding deployment")
	}
	// Phase-7 stub.
	out := make([][]float32, len(inputs))
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
