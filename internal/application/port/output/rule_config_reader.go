package output

import (
	"context"

	"relaybox/internal/domain"
)

// RuleConfigReader returns the rule and associated outputs for a given input type.
type RuleConfigReader interface {
	GetRule(ctx context.Context, inputType string) (domain.Rule, []domain.Output, error)
}
