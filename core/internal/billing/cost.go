// cost.go is the v0.2 stub of the Azure Cost Management client. v0.2.1 will
// wire the real ARM POST to /providers/Microsoft.CostManagement/query —
// see https://learn.microsoft.com/rest/api/cost-management/query for the
// expected request shape.
package billing

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotImplemented is the sentinel for the v0.2.0 stub.
var ErrNotImplemented = errors.New("billing.cost: not yet implemented (v0.2 scaffold)")

// CostRow is one row of the cost summary.
type CostRow struct {
	Date    string  `json:"date"`
	USD     float64 `json:"usd"`
	Service string  `json:"service"`
}

// CostClient is the typed wrapper. Construct with a control.LockedHTTPClient.
type CostClient struct {
	HTTP           *http.Client
	SubscriptionID string
	Token          func(ctx context.Context) (string, error)
}

// Summary returns daily cost rows for the trailing window. v0.2.0 stub.
func (c *CostClient) Summary(ctx context.Context, days int) ([]CostRow, error) {
	if c == nil || c.SubscriptionID == "" {
		return nil, fmt.Errorf("billing.cost: subscription_id is required")
	}
	if days <= 0 {
		return nil, fmt.Errorf("billing.cost: days must be > 0")
	}
	return nil, ErrNotImplemented
}
