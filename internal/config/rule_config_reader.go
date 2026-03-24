package config

import (
	"context"
	"fmt"
	"sync"

	"relaybox/internal/domain"
)

type inputEntry struct {
	engine  string
	entries []domain.RuleEntry
}

// InMemoryRuleConfigReader implements output.RuleConfigReader.
type InMemoryRuleConfigReader struct {
	mu     sync.RWMutex
	inputs map[string]inputEntry // keyed by input ID
}

func NewInMemoryRuleConfigReader(cfg *Config) *InMemoryRuleConfigReader {
	r := &InMemoryRuleConfigReader{}
	r.Update(cfg)
	return r
}

func (r *InMemoryRuleConfigReader) Update(cfg *Config) {
	outputsByID := make(map[string]domain.Output, len(cfg.Outputs))
	for _, c := range cfg.Outputs {
		outputsByID[c.ID] = domain.Output{
			ID: c.ID, Type: domain.OutputType(c.Type), Engine: c.Engine, URL: c.URL,
			Template: c.Template, Secret: c.Secret,
			RetryCount: c.RetryCount, RetryDelayMs: c.RetryDelayMs,
			TimeoutSec: c.TimeoutSec, SkipTLSVerify: c.SkipTLSVerify,
		}
	}

	inputs := make(map[string]inputEntry, len(cfg.Inputs))
	for _, inp := range cfg.Inputs {
		key := inp.ID

		entries := make([]domain.RuleEntry, 0, len(inp.Rules))
		for _, rc := range inp.Rules {
			// Build routing conditions
			var routing []domain.RouteCondition
			for _, rcond := range rc.Routing {
				routing = append(routing, domain.RouteCondition{
					Condition: rcond.Condition,
					OutputIDs: rcond.OutputIDs,
				})
			}

			rule := domain.Rule{
				Filter:    rc.Filter,
				Mapping:   rc.Mapping,
				Routing:   routing,
				OutputIDs: rc.OutputIDs,
			}

			// Collect outputs from outputIds and routing
			outputIDSet := make(map[string]struct{})
			for _, id := range rc.OutputIDs {
				outputIDSet[id] = struct{}{}
			}
			for _, rcond := range rc.Routing {
				for _, id := range rcond.OutputIDs {
					outputIDSet[id] = struct{}{}
				}
			}

			var outputs []domain.Output
			for id := range outputIDSet {
				if out, ok := outputsByID[id]; ok {
					outputs = append(outputs, out)
				}
			}

			entries = append(entries, domain.RuleEntry{Rule: rule, Outputs: outputs})
		}

		inputs[key] = inputEntry{engine: inp.Engine, entries: entries}
	}

	r.mu.Lock()
	r.inputs = inputs
	r.mu.Unlock()
}

// GetRules returns the input engine and rule entries for a given input ID.
func (r *InMemoryRuleConfigReader) GetRules(_ context.Context, inputID string) (string, []domain.RuleEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.inputs[inputID]
	if !ok {
		return "", nil, fmt.Errorf("rules for %q: %w", inputID, domain.ErrInputNotFound)
	}
	return entry.engine, entry.entries, nil
}
