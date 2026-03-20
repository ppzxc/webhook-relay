package output

import (
	"context"
	"time"

	"relaybox/internal/domain"
)

type MessageRepository interface {
	Save(ctx context.Context, msg domain.Message) error
	UpdateDeliveryState(ctx context.Context, id string, status domain.MessageStatus, retryCount int, lastAttemptAt time.Time) error
	FindByID(ctx context.Context, id string) (domain.Message, error)
	FindByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error)
}
