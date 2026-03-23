package expression

import (
	"fmt"

	"relaybox/internal/application/port/output"
)

// InMemoryExpressionEngineRegistry stores expression engines by type.
type InMemoryExpressionEngineRegistry struct {
	engines map[string]output.ExpressionEngine
}

// NewInMemoryExpressionEngineRegistry creates an empty registry.
func NewInMemoryExpressionEngineRegistry() *InMemoryExpressionEngineRegistry {
	return &InMemoryExpressionEngineRegistry{
		engines: make(map[string]output.ExpressionEngine),
	}
}

// Register adds an engine.
func (r *InMemoryExpressionEngineRegistry) Register(engine output.ExpressionEngine) {
	r.engines[engine.Type()] = engine
}

// Get returns the engine with the given type.
func (r *InMemoryExpressionEngineRegistry) Get(engineType string) (output.ExpressionEngine, error) {
	e, ok := r.engines[engineType]
	if !ok {
		return nil, fmt.Errorf("expression engine %q not registered", engineType)
	}
	return e, nil
}
