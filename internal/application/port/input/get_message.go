package input

import (
	"context"
	"relaybox/internal/domain"
)

// GetMessageUseCase retrieves a single message by its ID.
type GetMessageUseCase interface {
	GetByID(ctx context.Context, id string) (domain.Message, error)
}
