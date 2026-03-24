package input

import (
	"context"

	"relaybox/internal/domain"
)

// ListMessagesUseCase lists messages for a given input with pagination.
type ListMessagesUseCase interface {
	ListByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error)
}
