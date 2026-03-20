package output

import (
	"context"

	"webhook-relay/internal/domain"
)

type RouteConfigReader interface {
	GetChannels(ctx context.Context, sourceID string) ([]domain.Channel, error)
}
