package output

import (
	"context"
	"time"

	"webhook-relay/internal/domain"
)

type AlertRepository interface {
	Save(ctx context.Context, alert domain.Alert) error
	UpdateDeliveryState(ctx context.Context, id string, status domain.AlertStatus, retryCount int, lastAttemptAt time.Time) error
	FindByID(ctx context.Context, id string) (domain.Alert, error)
	FindBySource(ctx context.Context, sourceID string, limit, offset int) ([]domain.Alert, error)
}
