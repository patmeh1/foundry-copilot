package control

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClient_BaseURL_RequiresAllFields(t *testing.T) {
	cases := []struct {
		name string
		c    Client
	}{
		{"nil sub", Client{ResourceGroup: "rg", AccountName: "a"}},
		{"nil rg", Client{SubscriptionID: "s", AccountName: "a"}},
		{"nil acct", Client{SubscriptionID: "s", ResourceGroup: "rg"}},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.c.baseURL()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestClient_BaseURL_ValidPassesValidateEndpoint(t *testing.T) {
	c := Client{
		SubscriptionID: "00000000-0000-0000-0000-000000000000",
		ResourceGroup:  "rg",
		AccountName:    "acct",
	}
	got, err := c.baseURL()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "management.azure.com") {
		t.Fatalf("baseURL %q must contain management.azure.com", got)
	}
}

func TestClient_List_NotImplemented(t *testing.T) {
	c := Client{SubscriptionID: "s", ResourceGroup: "rg", AccountName: "a"}
	_, err := c.List(context.Background(), "")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestClient_Get_NotImplemented(t *testing.T) {
	c := Client{SubscriptionID: "s", ResourceGroup: "rg", AccountName: "a"}
	_, err := c.Get(context.Background(), "my-dep")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestClient_Get_RequiresName(t *testing.T) {
	c := Client{SubscriptionID: "s", ResourceGroup: "rg", AccountName: "a"}
	_, err := c.Get(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("want 'name is required', got %v", err)
	}
}
