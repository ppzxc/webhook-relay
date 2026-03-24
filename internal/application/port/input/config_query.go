package input

import "context"

// ConfigQueryUseCase exposes read-only config metadata (inputs and outputs).
type ConfigQueryUseCase interface {
	ListInputs(ctx context.Context) ([]InputSummary, error)
	GetInput(ctx context.Context, id string) (InputSummary, error)
	ListOutputs(ctx context.Context) ([]OutputSummary, error)
	GetOutput(ctx context.Context, id string) (OutputSummary, error)
}

// InputSummary is the public representation of a configured input.
// Secrets are never included.
type InputSummary struct {
	ID string `json:"id"`
}

// OutputSummary is the public representation of a configured output.
// Secrets and internal fields are never included.
// URL is intentionally included for observability — no auth credentials are embedded.
type OutputSummary struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	RetryCount   int    `json:"retryCount"`
	RetryDelayMs int    `json:"retryDelayMs"`
}
