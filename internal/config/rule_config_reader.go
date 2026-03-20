package config

import (
	"context"
	"fmt"
	"sync"

	"relaybox/internal/domain"
)

type InMemoryRuleConfigReader struct {
	mu      sync.RWMutex
	outputs map[string]domain.Output
	rules   map[string][]string
}

func NewInMemoryRuleConfigReader(cfg *Config) *InMemoryRuleConfigReader {
	r := &InMemoryRuleConfigReader{}
	r.Update(cfg)
	return r
}

func (r *InMemoryRuleConfigReader) Update(cfg *Config) {
	outputs := make(map[string]domain.Output, len(cfg.Outputs))
	for _, c := range cfg.Outputs {
		outputs[c.ID] = domain.Output{
			ID: c.ID, Type: domain.OutputType(c.Type), URL: c.URL,
			Template: c.Template, RetryCount: c.RetryCount,
			RetryDelayMs: c.RetryDelayMs, TimeoutSec: c.TimeoutSec,
			SkipTLSVerify: c.SkipTLSVerify,
		}
	}
	// inputID → inputType mapping (e.g. "beszel" → "BESZEL")
	inputTypeByID := make(map[string]string, len(cfg.Inputs))
	for _, s := range cfg.Inputs {
		inputTypeByID[s.ID] = s.Type
	}
	// Rules keyed by input type so relay worker can query with msg.Input
	rules := make(map[string][]string, len(cfg.Rules))
	for _, rt := range cfg.Rules {
		key := inputTypeByID[rt.InputID]
		if key == "" {
			key = rt.InputID // fallback
		}
		rules[key] = rt.OutputIDs
	}
	r.mu.Lock()
	r.outputs = outputs
	r.rules = rules
	r.mu.Unlock()
}

func (r *InMemoryRuleConfigReader) GetOutputs(_ context.Context, inputID string) ([]domain.Output, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids, ok := r.rules[inputID]
	if !ok {
		return nil, fmt.Errorf("rule for %q: %w", inputID, domain.ErrInputNotFound)
	}
	result := make([]domain.Output, 0, len(ids))
	for _, id := range ids {
		if out, ok := r.outputs[id]; ok {
			result = append(result, out)
		}
	}
	return result, nil
}
