package foundryagents

import (
	"context"
	"errors"
	"testing"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
)

func TestNewClient_RejectsNonFoundryEndpoint(t *testing.T) {
	cases := []string{
		"https://api.openai.com/v1",
		"https://api.anthropic.com/v1/messages",
		"http://localhost:11434",
		"https://management.azure.com/", // control-plane is NOT data-plane
		"",
	}
	for _, ep := range cases {
		ep := ep
		t.Run(ep, func(t *testing.T) {
			_, err := NewClient(ep, nil, nil)
			if err == nil {
				t.Fatalf("expected error for endpoint %q", ep)
			}
			if !errors.Is(err, foundry.ErrEndpointNotFoundry) {
				t.Fatalf("error must wrap foundry.ErrEndpointNotFoundry, got %v", err)
			}
		})
	}
}

func TestNewClient_AcceptsFoundryEndpoint(t *testing.T) {
	c, err := NewClient("https://myproj.services.ai.azure.com/", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("client should not be nil")
	}
	if c.HTTP == nil {
		t.Fatal("HTTP client should default to foundry.LockedHTTPClient")
	}
}

func TestCreateThread_NotImplemented(t *testing.T) {
	c, _ := NewClient("https://myproj.services.ai.azure.com/", nil, nil)
	_, err := c.CreateThread(context.Background())
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestPostMessage_ValidatesRole(t *testing.T) {
	c, _ := NewClient("https://myproj.services.ai.azure.com/", nil, nil)
	_, err := c.PostMessage(context.Background(), "th_1", "bogus", "hi")
	if err == nil {
		t.Fatal("expected role validation error")
	}
}

func TestStartRun_ValidatesArgs(t *testing.T) {
	c, _ := NewClient("https://myproj.services.ai.azure.com/", nil, nil)
	if _, err := c.StartRun(context.Background(), "", "ag_1"); err == nil {
		t.Fatal("expected error for empty threadID")
	}
	if _, err := c.StartRun(context.Background(), "th_1", ""); err == nil {
		t.Fatal("expected error for empty agentID")
	}
}
