package input

import (
	"context"

	"relaybox/internal/domain"
)

// RequeueMessageUseCase transitions a FAILED message back to PENDING and re-enqueues it.
type RequeueMessageUseCase interface {
	Requeue(ctx context.Context, messageID string) (domain.Message, error)
}
