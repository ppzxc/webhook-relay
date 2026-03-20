package output

import (
	"context"

	"relaybox/internal/domain"
)

type RuleConfigReader interface {
	GetOutputs(ctx context.Context, inputID string) ([]domain.Output, error)
}
