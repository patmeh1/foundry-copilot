// Package billing parses Foundry rate-limit response headers, manages a
// daily budget alert, and (in v0.2.1) queries Azure Cost Management via the
// control-plane LockedHTTPClient.
//
// v0.2.0 ships: header parser (fully implemented + tested), budget alerter
// (fully implemented + tested), and a CostClient stub gated on
// SubscriptionID.
package billing

import (
	"net/http"
	"strconv"
)

// Quota captures Foundry's per-deployment rate-limit headers from one HTTP
// response. Any missing/malformed header value is zero; callers may treat 0
// as "unknown".
type Quota struct {
	Remaining int   // x-ratelimit-remaining
	Limit     int   // x-ratelimit-limit
	ResetAt   int64 // x-ratelimit-reset-at (unix seconds)
}

// ParseQuota extracts the three known rate-limit headers.
func ParseQuota(h http.Header) Quota {
	return Quota{
		Remaining: atoiOr(h.Get("x-ratelimit-remaining"), 0),
		Limit:     atoiOr(h.Get("x-ratelimit-limit"), 0),
		ResetAt:   atoi64Or(h.Get("x-ratelimit-reset-at"), 0),
	}
}

func atoiOr(s string, d int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return d
	}
	return n
}

func atoi64Or(s string, d int64) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return d
	}
	return n
}
