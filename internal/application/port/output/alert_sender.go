package output

import (
	"context"

	"webhook-relay/internal/domain"
)

type AlertSender interface {
	Send(ctx context.Context, channel domain.Channel, alert domain.Alert) error
}
