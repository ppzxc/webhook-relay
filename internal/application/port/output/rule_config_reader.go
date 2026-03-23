package output

import (
	"context"

	"relaybox/internal/domain"
)

// RuleConfigReader returns the input engine and rule entries for a given input type.
type RuleConfigReader interface {
	GetRules(ctx context.Context, inputType string) (inputEngine string, entries []domain.RuleEntry, err error)
}
