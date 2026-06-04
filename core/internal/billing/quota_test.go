package billing

import (
	"net/http"
	"testing"
)

func TestParseQuota_Full(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-remaining", "499")
	h.Set("x-ratelimit-limit", "500")
	h.Set("x-ratelimit-reset-at", "1730000000")
	got := ParseQuota(h)
	if got.Remaining != 499 || got.Limit != 500 || got.ResetAt != 1730000000 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseQuota_Missing(t *testing.T) {
	got := ParseQuota(http.Header{})
	if got.Remaining != 0 || got.Limit != 0 || got.ResetAt != 0 {
		t.Fatalf("expected zero-values, got %+v", got)
	}
}

func TestParseQuota_Malformed(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-remaining", "not-a-number")
	h.Set("x-ratelimit-limit", "")
	got := ParseQuota(h)
	if got.Remaining != 0 || got.Limit != 0 {
		t.Fatalf("expected zero-values on malformed input, got %+v", got)
	}
}
