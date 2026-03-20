package expression_test

import (
	"testing"

	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/application/port/output"
)

var _ output.ExpressionEngine = (*expression.CELEngine)(nil)

func TestCELEngine_Type(t *testing.T) {
	e := expression.NewCELEngine()
	if e.Type() != "cel" {
		t.Errorf("Type() = %q, want cel", e.Type())
	}
}

func TestCELEngine_EvaluateString(t *testing.T) {
	e := expression.NewCELEngine()
	data := map[string]any{"name": "world"}
	got, err := e.EvaluateString(`"hello " + name`, data)
	if err != nil {
		t.Fatalf("EvaluateString error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestCELEngine_EvaluateBool(t *testing.T) {
	e := expression.NewCELEngine()
	data := map[string]any{"status": "CRITICAL"}

	got, err := e.EvaluateBool(`status == "CRITICAL"`, data)
	if err != nil {
		t.Fatalf("EvaluateBool error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}

	got, err = e.EvaluateBool(`status == "OK"`, data)
	if err != nil {
		t.Fatalf("EvaluateBool error: %v", err)
	}
	if got {
		t.Error("expected false")
	}
}

func TestCELEngine_EvaluateNumeric(t *testing.T) {
	e := expression.NewCELEngine()
	data := map[string]any{"a": int64(10), "b": int64(20)}
	got, err := e.Evaluate(`a + b`, data)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if got != int64(30) {
		t.Errorf("got %v (%T), want 30", got, got)
	}
}

func TestCELEngine_CacheHit(t *testing.T) {
	e := expression.NewCELEngine()
	data := map[string]any{"x": "a"}
	// First call compiles; second should use cache
	for i := range 3 {
		got, err := e.EvaluateString(`x`, data)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if got != "a" {
			t.Errorf("iteration %d: got %q", i, got)
		}
	}
}

func TestCELEngine_ErrorInvalidExpr(t *testing.T) {
	e := expression.NewCELEngine()
	_, err := e.Evaluate(`!!!`, map[string]any{"x": "a"})
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestCELEngine_EvaluateBool_TypeMismatch(t *testing.T) {
	e := expression.NewCELEngine()
	_, err := e.EvaluateBool(`"hello"`, map[string]any{})
	if err == nil {
		t.Error("expected error when result is not bool")
	}
}

func TestCELEngine_EvaluateString_TypeMismatch(t *testing.T) {
	e := expression.NewCELEngine()
	_, err := e.EvaluateString(`true`, map[string]any{})
	if err == nil {
		t.Error("expected error when result is not string")
	}
}

func TestCELEngine_MapData(t *testing.T) {
	e := expression.NewCELEngine()
	data := map[string]any{
		"payload": `{"host":"server1"}`,
		"input":   "BESZEL",
	}
	got, err := e.EvaluateString(`input + ": " + payload`, data)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := `BESZEL: {"host":"server1"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
