package finetune

import (
	"context"
	"errors"
	"testing"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
)

func TestNewJobsClient_RejectsNonFoundryEndpoint(t *testing.T) {
	for _, ep := range []string{"https://api.openai.com/v1", "http://localhost", ""} {
		_, err := NewJobsClient(ep, nil, nil)
		if err == nil || !errors.Is(err, foundry.ErrEndpointNotFoundry) {
			t.Fatalf("ep=%q want ErrEndpointNotFoundry, got %v", ep, err)
		}
	}
}

func TestNewJobsClient_AcceptsFoundryEndpoint(t *testing.T) {
	c, err := NewJobsClient("https://myproj.services.ai.azure.com/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTP == nil {
		t.Fatal("HTTP should default to foundry.LockedHTTPClient")
	}
}

func TestCreateJob_NotImplemented(t *testing.T) {
	c, _ := NewJobsClient("https://myproj.services.ai.azure.com/", nil, nil)
	_, err := c.CreateJob(context.Background(), "gpt-4o-mini", "file_123")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestCreateJob_ValidatesArgs(t *testing.T) {
	c, _ := NewJobsClient("https://myproj.services.ai.azure.com/", nil, nil)
	if _, err := c.CreateJob(context.Background(), "", "f"); err == nil {
		t.Fatal("expected validation error")
	}
}
