// jobs.go is the v0.2 stub of the Foundry fine-tuning REST client. The
// constructor enforces the data-plane allow-list (any non-Foundry endpoint
// is rejected with foundry.ErrEndpointNotFoundry). v0.2.1 will wire the
// real REST calls; v0.2.0 returns ErrNotImplemented so the RPC surface +
// dataset builder can be tested end-to-end today.
package finetune

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/patmeh1/foundry-copilot/core/internal/foundry"
)

// ErrNotImplemented is the sentinel returned by every stub method.
var ErrNotImplemented = errors.New("finetune: not yet implemented (v0.2 scaffold)")

// Job is one fine-tuning job's typed state.
type Job struct {
	ID             string `json:"id"`
	Model          string `json:"model"`
	TrainingFile   string `json:"training_file"`
	Status         string `json:"status"`
	CreatedAt      int64  `json:"created_at"`
	FineTunedModel string `json:"fine_tuned_model,omitempty"`
}

// JobsClient is the typed REST client. Construct via NewJobsClient.
type JobsClient struct {
	HTTP     *http.Client
	Endpoint string
	Token    func(ctx context.Context) (string, error)
}

// NewJobsClient enforces the Foundry endpoint allow-list at construction.
func NewJobsClient(endpoint string, httpClient *http.Client, token func(ctx context.Context) (string, error)) (*JobsClient, error) {
	if err := foundry.ValidateEndpoint(endpoint); err != nil {
		return nil, fmt.Errorf("finetune: %w", err)
	}
	if httpClient == nil {
		httpClient = foundry.LockedHTTPClient()
	}
	return &JobsClient{HTTP: httpClient, Endpoint: endpoint, Token: token}, nil
}

// UploadDataset uploads a JSONL training file. v0.2.0 stub.
func (c *JobsClient) UploadDataset(ctx context.Context, name string, data []byte) (string, error) {
	if name == "" || len(data) == 0 {
		return "", fmt.Errorf("finetune.UploadDataset: name and data are required")
	}
	return "", ErrNotImplemented
}

// CreateJob starts a fine-tuning job from an uploaded file. v0.2.0 stub.
func (c *JobsClient) CreateJob(ctx context.Context, model, fileID string) (Job, error) {
	if model == "" || fileID == "" {
		return Job{}, fmt.Errorf("finetune.CreateJob: model and fileID are required")
	}
	return Job{}, ErrNotImplemented
}

// GetJob polls the status of an existing job. v0.2.0 stub.
func (c *JobsClient) GetJob(ctx context.Context, id string) (Job, error) {
	return Job{}, ErrNotImplemented
}

// ListJobs lists fine-tuning jobs. v0.2.0 stub.
func (c *JobsClient) ListJobs(ctx context.Context) ([]Job, error) {
	return nil, ErrNotImplemented
}

// CancelJob cancels a running job. v0.2.0 stub.
func (c *JobsClient) CancelJob(ctx context.Context, id string) error {
	return ErrNotImplemented
}
