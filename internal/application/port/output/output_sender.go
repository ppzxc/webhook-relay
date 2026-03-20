package output

import (
	"context"

	"relaybox/internal/domain"
)

type OutputSender interface {
	Send(ctx context.Context, out domain.Output, msg domain.Message) error
}
