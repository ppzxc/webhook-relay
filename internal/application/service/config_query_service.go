package service

import (
	"context"
	"fmt"
	"sync"

	inputport "relaybox/internal/application/port/input"
	cfgpkg "relaybox/internal/config"
	"relaybox/internal/domain"
)

// Compile-time interface check
var _ inputport.ConfigQueryUseCase = (*ConfigQueryService)(nil)

// ConfigQueryService exposes read-only config metadata for inputs and outputs.
// It is safe for concurrent use and supports hot-reload via Update.
type ConfigQueryService struct {
	mu          sync.RWMutex
	inputs      []inputport.InputSummary
	outputs     []inputport.OutputSummary
	inputsByID  map[string]inputport.InputSummary
	outputsByID map[string]inputport.OutputSummary
}

func NewConfigQueryService(cfg *cfgpkg.Config) *ConfigQueryService {
	s := &ConfigQueryService{}
	s.Update(cfg)
	return s
}

// Update rebuilds the in-memory snapshots from a (possibly hot-reloaded) config.
func (s *ConfigQueryService) Update(cfg *cfgpkg.Config) {
	inputs := make([]inputport.InputSummary, 0, len(cfg.Inputs))
	inputsByID := make(map[string]inputport.InputSummary, len(cfg.Inputs))
	for _, c := range cfg.Inputs {
		summary := inputport.InputSummary{ID: c.ID}
		inputs = append(inputs, summary)
		inputsByID[c.ID] = summary
	}

	outputs := make([]inputport.OutputSummary, 0, len(cfg.Outputs))
	outputsByID := make(map[string]inputport.OutputSummary, len(cfg.Outputs))
	for _, c := range cfg.Outputs {
		summary := inputport.OutputSummary{
			ID:           c.ID,
			Type:         c.Type,
			URL:          c.URL,
			RetryCount:   c.RetryCount,
			RetryDelayMs: c.RetryDelayMs,
		}
		outputs = append(outputs, summary)
		outputsByID[c.ID] = summary
	}

	s.mu.Lock()
	s.inputs = inputs
	s.outputs = outputs
	s.inputsByID = inputsByID
	s.outputsByID = outputsByID
	s.mu.Unlock()
}

func (s *ConfigQueryService) ListInputs(_ context.Context) ([]inputport.InputSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]inputport.InputSummary, len(s.inputs))
	copy(result, s.inputs)
	return result, nil
}

func (s *ConfigQueryService) GetInput(_ context.Context, id string) (inputport.InputSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inp, ok := s.inputsByID[id]
	if !ok {
		return inputport.InputSummary{}, fmt.Errorf("get input %q: %w", id, domain.ErrInputNotFound)
	}
	return inp, nil
}

func (s *ConfigQueryService) ListOutputs(_ context.Context) ([]inputport.OutputSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]inputport.OutputSummary, len(s.outputs))
	copy(result, s.outputs)
	return result, nil
}

func (s *ConfigQueryService) GetOutput(_ context.Context, id string) (inputport.OutputSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out, ok := s.outputsByID[id]
	if !ok {
		return inputport.OutputSummary{}, fmt.Errorf("get output %q: %w", id, domain.ErrOutputNotFound)
	}
	return out, nil
}
