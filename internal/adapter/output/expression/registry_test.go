package expression_test

import (
	"testing"

	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/application/port/output"
)

var _ output.ExpressionEngineRegistry = (*expression.InMemoryExpressionEngineRegistry)(nil)

func TestRegistry_GetAndDefault(t *testing.T) {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	celEng := expression.NewCELEngine()
	exprEng := expression.NewExprEngine()

	reg.Register(celEng)
	reg.Register(exprEng)

	// Default is first registered
	if reg.Default().Type() != "cel" {
		t.Errorf("Default() = %q, want cel", reg.Default().Type())
	}

	got, err := reg.Get("expr")
	if err != nil {
		t.Fatalf("Get(expr) error: %v", err)
	}
	if got.Type() != "expr" {
		t.Errorf("Get(expr).Type() = %q", got.Type())
	}

	_, err = reg.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown engine")
	}
}

func TestRegistry_SetDefault(t *testing.T) {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	reg.Register(expression.NewCELEngine())
	reg.Register(expression.NewExprEngine())

	if err := reg.SetDefault("expr"); err != nil {
		t.Fatalf("SetDefault error: %v", err)
	}
	if reg.Default().Type() != "expr" {
		t.Errorf("Default() = %q, want expr", reg.Default().Type())
	}

	if err := reg.SetDefault("nonexistent"); err == nil {
		t.Error("expected error for nonexistent engine")
	}
}

func TestRegistry_EmptyDefault(t *testing.T) {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	if reg.Default() != nil {
		t.Error("Default() on empty registry should be nil")
	}
}
