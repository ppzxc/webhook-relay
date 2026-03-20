package parser

import (
	"errors"
	"fmt"

	"relaybox/internal/application/port/input"
)

var ErrParserNotFound = errors.New("parser not found")

// InMemoryParserRegistry holds parsers in memory indexed by type name.
type InMemoryParserRegistry struct {
	parsers map[string]input.Parser
}

func NewInMemoryParserRegistry() *InMemoryParserRegistry {
	return &InMemoryParserRegistry{parsers: make(map[string]input.Parser)}
}

func (r *InMemoryParserRegistry) Register(p input.Parser) {
	r.parsers[p.Type()] = p
}

// RegisterWithKey registers a parser under an explicit key rather than its Type().
// Useful for parsers that need per-instance keys, such as regex parsers with unique patterns.
func (r *InMemoryParserRegistry) RegisterWithKey(key string, p input.Parser) {
	r.parsers[key] = p
}

func (r *InMemoryParserRegistry) Get(parserType string) (input.Parser, error) {
	p, ok := r.parsers[parserType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrParserNotFound, parserType)
	}
	return p, nil
}
