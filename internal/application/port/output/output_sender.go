package output

import (
	"context"

	"relaybox/internal/domain"
)

// OutputSender sends a pre-rendered payload to an output destination.
type OutputSender interface {
	Send(ctx context.Context, out domain.Output, payload []byte) error
}
