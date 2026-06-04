// Package foundryagents is the v0.2 client for the Microsoft Foundry
// Agents Service (Threads/Messages/Runs REST API).
//
// v0.2.0 is a stub: the constructor enforces the Foundry endpoint allow-list
// via core/internal/foundry.ValidateEndpoint, and all methods return
// ErrNotImplemented. The wire format and signatures are stable so the
// extension's "Use hosted Foundry agents" toggle (opt-in, default off) can
// be wired now and exercised once v0.2.1 ships the real REST round-trippers.
package foundryagents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
)

// ErrNotImplemented is the sentinel returned by every method on the v0.2.0 stub.
var ErrNotImplemented = errors.New("foundryagents: not yet implemented (v0.2 scaffold)")

// Thread is a conversation handle returned by the Agents Service.
type Thread struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created_at"`
}

// Message is one entry in a Thread.
type Message struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Run is an Agents Service execution against a Thread.
type Run struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Client is the typed REST client. Construct via NewClient — the constructor
// enforces the data-plane allow-list so misconfigured endpoints never get a
// chance to issue traffic.
type Client struct {
	HTTP     *http.Client
	Endpoint string
	Token    func(ctx context.Context) (string, error)
}

// NewClient returns a *Client only if endpoint passes foundry.ValidateEndpoint.
// Any other host (api.openai.com, localhost, IP literal, etc.) is rejected
// with an error that wraps foundry.ErrEndpointNotFoundry.
func NewClient(endpoint string, httpClient *http.Client, token func(ctx context.Context) (string, error)) (*Client, error) {
	if err := foundry.ValidateEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("foundryagents: %w", err)
	}
	if httpClient == nil {
		httpClient = foundry.LockedHTTPClient()
	}
	return &Client{HTTP: httpClient, Endpoint: endpoint, Token: token}, nil
}

// CreateThread starts a new Agents Service thread. v0.2.0 stub.
func (c *Client) CreateThread(ctx context.Context) (Thread, error) {
	return Thread{}, ErrNotImplemented
}

// PostMessage appends a message to an existing thread. v0.2.0 stub.
func (c *Client) PostMessage(ctx context.Context, threadID, role, content string) (Message, error) {
	if threadID == "" {
		return Message{}, fmt.Errorf("foundryagents.PostMessage: threadID is required")
	}
	if role != "user" && role != "assistant" && role != "system" {
		return Message{}, fmt.Errorf("foundryagents.PostMessage: role must be user|assistant|system, got %q", role)
	}
	return Message{}, ErrNotImplemented
}

// StartRun creates a Run on a thread for the given agent. v0.2.0 stub.
func (c *Client) StartRun(ctx context.Context, threadID, agentID string) (Run, error) {
	if threadID == "" || agentID == "" {
		return Run{}, fmt.Errorf("foundryagents.StartRun: threadID and agentID are required")
	}
	return Run{}, ErrNotImplemented
}

// GetRun polls the status of an existing Run. v0.2.0 stub.
func (c *Client) GetRun(ctx context.Context, threadID, runID string) (Run, error) {
	return Run{}, ErrNotImplemented
}

// ListMessages returns the messages on a thread. v0.2.0 stub.
func (c *Client) ListMessages(ctx context.Context, threadID string) ([]Message, error) {
	return nil, ErrNotImplemented
}
