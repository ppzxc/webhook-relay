package expression_test

import (
	"testing"

	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/application/port/output"
)

var _ output.ExpressionEngineRegistry = (*expression.InMemoryExpressionEngineRegistry)(nil)

func TestRegistry_Get(t *testing.T) {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	celEng := newCELEngine(t)
	exprEng := expression.NewExprEngine()

	reg.Register(celEng)
	reg.Register(exprEng)

	got, err := reg.Get("CEL")
	if err != nil {
		t.Fatalf("Get(CEL) error: %v", err)
	}
	if got.Type() != "CEL" {
		t.Errorf("Get(CEL).Type() = %q, want CEL", got.Type())
	}

	got, err = reg.Get("EXPR")
	if err != nil {
		t.Fatalf("Get(EXPR) error: %v", err)
	}
	if got.Type() != "EXPR" {
		t.Errorf("Get(EXPR).Type() = %q, want EXPR", got.Type())
	}

	_, err = reg.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown engine")
	}
}
