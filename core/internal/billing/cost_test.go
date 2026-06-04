package billing

import (
	"context"
	"errors"
	"testing"
)

func TestSummary_RequiresSubscription(t *testing.T) {
	c := &CostClient{}
	_, err := c.Summary(context.Background(), 30)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSummary_RequiresPositiveDays(t *testing.T) {
	c := &CostClient{SubscriptionID: "00000000-0000-0000-0000-000000000000"}
	_, err := c.Summary(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSummary_NotImplemented(t *testing.T) {
	c := &CostClient{SubscriptionID: "00000000-0000-0000-0000-000000000000"}
	_, err := c.Summary(context.Background(), 30)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}
