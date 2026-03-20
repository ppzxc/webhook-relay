package config

import (
	"context"
	"fmt"
	"sync"

	"relaybox/internal/domain"
)

type ruleEntry struct {
	rule    domain.Rule
	outputs []domain.Output
}

// InMemoryRuleConfigReader implements output.RuleConfigReader.
type InMemoryRuleConfigReader struct {
	mu    sync.RWMutex
	rules map[string]ruleEntry // keyed by input type (e.g. "BESZEL")
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
			ID: c.ID, Type: domain.OutputType(c.Type), URL: c.URL,
			Template: c.Template, Secret: c.Secret,
			RetryCount: c.RetryCount, RetryDelayMs: c.RetryDelayMs,
			TimeoutSec: c.TimeoutSec, SkipTLSVerify: c.SkipTLSVerify,
		}
	}

	// inputID -> inputType mapping (e.g. "beszel" -> "BESZEL")
	inputTypeByID := make(map[string]string, len(cfg.Inputs))
	for _, s := range cfg.Inputs {
		inputTypeByID[s.ID] = s.Type
	}

	rules := make(map[string]ruleEntry, len(cfg.Rules))
	for _, rt := range cfg.Rules {
		key := inputTypeByID[rt.InputID]
		if key == "" {
			key = rt.InputID // fallback
		}

		// Build routing conditions
		var routing []domain.RouteCondition
		for _, rc := range rt.Routing {
			routing = append(routing, domain.RouteCondition{
				Condition: rc.Condition,
				OutputIDs: rc.OutputIDs,
			})
		}

		rule := domain.Rule{
			InputID: rt.InputID,
			Engine:  rt.Engine,
			Filter:  rt.Filter,
			Mapping: rt.Mapping,
			Routing: routing,
		}

		// Collect outputs from outputIds (backward compat) and routing
		outputIDSet := make(map[string]struct{})
		for _, id := range rt.OutputIDs {
			outputIDSet[id] = struct{}{}
		}
		for _, rc := range rt.Routing {
			for _, id := range rc.OutputIDs {
				outputIDSet[id] = struct{}{}
			}
		}

		var outputs []domain.Output
		for id := range outputIDSet {
			if out, ok := outputsByID[id]; ok {
				outputs = append(outputs, out)
			}
		}

		rules[key] = ruleEntry{rule: rule, outputs: outputs}
	}

	r.mu.Lock()
	r.rules = rules
	r.mu.Unlock()
}

// GetRule returns the rule and associated outputs for a given input type.
func (r *InMemoryRuleConfigReader) GetRule(_ context.Context, inputType string) (domain.Rule, []domain.Output, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.rules[inputType]
	if !ok {
		return domain.Rule{}, nil, fmt.Errorf("rule for %q: %w", inputType, domain.ErrInputNotFound)
	}
	return entry.rule, entry.outputs, nil
}
