// deployments.go is the v0.2 stub of the Foundry deployments control-plane
// client. The full ARM REST integration lands in Phase 14 verification; for
// v0.2.0 scaffolding the methods return ErrNotImplemented so the RPC surface,
// tree-view, and tests can be wired end-to-end now.
//
// All methods are required to validate their target URL via ValidateEndpoint
// before issuing any HTTP call. This is enforced by the LockedHTTPClient
// transport, but client code MUST also call ValidateEndpoint up-front so the
// error surfaces with the deployment ID context instead of a generic transport
// rejection.
package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotImplemented is returned by stub methods on the v0.2.0 scaffold.
// Replace with full ARM HTTP calls in v0.2.1.
var ErrNotImplemented = errors.New("control.Client: not yet implemented (Phase 14 scaffold)")

// Deployment is the shape returned to the extension via deploy/* RPCs.
type Deployment struct {
	Name         string   `json:"name"`
	Model        string   `json:"model"`
	SKU          string   `json:"sku"`
	Capacity     int      `json:"capacity"`
	State        string   `json:"state"`
	ETag         string   `json:"etag,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// Client is the ARM deployments client. Construct via:
//
//	&control.Client{
//	    HTTP:           control.LockedHTTPClient(),
//	    SubscriptionID: "...",
//	    ResourceGroup:  "...",
//	    AccountName:    "...",
//	    Token:          func(ctx context.Context) (string, error) { ... },
//	}
type Client struct {
	HTTP           *http.Client
	SubscriptionID string
	ResourceGroup  string
	AccountName    string
	Token          func(ctx context.Context) (string, error)
}

// baseURL constructs the ARM resource URL for the configured account and
// returns it only after ValidateEndpoint accepts it. Any missing field is
// reported as an error so the user sees an actionable message in the UI.
func (c *Client) baseURL() (string, error) {
	if c == nil {
		return "", fmt.Errorf("control.Client: nil receiver")
	}
	if c.SubscriptionID == "" {
		return "", fmt.Errorf("control.Client: SubscriptionID is required")
	}
	if c.ResourceGroup == "" {
		return "", fmt.Errorf("control.Client: ResourceGroup is required")
	}
	if c.AccountName == "" {
		return "", fmt.Errorf("control.Client: AccountName is required")
	}
	u := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.CognitiveServices/accounts/%s",
		c.SubscriptionID, c.ResourceGroup, c.AccountName,
	)
	if err := ValidateEndpoint(u); err != nil {
		return "", err
	}
	return u, nil
}

// List returns all deployments under the configured account. Phase 14.
func (c *Client) List(ctx context.Context, filterCapability string) ([]Deployment, error) {
	if _, err := c.baseURL(); err != nil {
		return nil, err
	}
	return nil, ErrNotImplemented
}

// Get fetches a single deployment by name. Phase 14.
func (c *Client) Get(ctx context.Context, name string) (Deployment, error) {
	if _, err := c.baseURL(); err != nil {
		return Deployment{}, err
	}
	if name == "" {
		return Deployment{}, fmt.Errorf("control.Client.Get: name is required")
	}
	return Deployment{}, ErrNotImplemented
}

// Create issues a PUT to ARM to create a deployment. Phase 14.
func (c *Client) Create(ctx context.Context, name, model, sku string, capacity int) (Deployment, error) {
	if _, err := c.baseURL(); err != nil {
		return Deployment{}, err
	}
	if name == "" || model == "" || sku == "" {
		return Deployment{}, fmt.Errorf("control.Client.Create: name, model, sku are required")
	}
	return Deployment{}, ErrNotImplemented
}

// Update changes the capacity of an existing deployment. Phase 14.
func (c *Client) Update(ctx context.Context, name string, capacity int) (Deployment, error) {
	if _, err := c.baseURL(); err != nil {
		return Deployment{}, err
	}
	return Deployment{}, ErrNotImplemented
}

// Delete removes a deployment. Phase 14.
func (c *Client) Delete(ctx context.Context, name string) error {
	if _, err := c.baseURL(); err != nil {
		return err
	}
	return ErrNotImplemented
}

// Test performs a 1-token completion against the deployment to verify auth
// and capacity assignment end-to-end. Phase 14.
func (c *Client) Test(ctx context.Context, name string) error {
	if _, err := c.baseURL(); err != nil {
		return err
	}
	return ErrNotImplemented
}
