package expression_test

import (
	"testing"

	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/application/port/output"
)

var _ output.ExpressionEngine = (*expression.ExprEngine)(nil)

func TestExprEngine_Type(t *testing.T) {
	e := expression.NewExprEngine()
	if e.Type() != "EXPR" {
		t.Errorf("Type() = %q, want EXPR", e.Type())
	}
}

func TestExprEngine_EvaluateString(t *testing.T) {
	e := expression.NewExprEngine()
	data := map[string]any{"name": "world"}
	got, err := e.EvaluateString(`"hello " + data.name`, data)
	if err != nil {
		t.Fatalf("EvaluateString error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExprEngine_EvaluateBool(t *testing.T) {
	e := expression.NewExprEngine()
	data := map[string]any{"status": "CRITICAL"}

	got, err := e.EvaluateBool(`data.status == "CRITICAL"`, data)
	if err != nil {
		t.Fatalf("EvaluateBool error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}

	got, err = e.EvaluateBool(`data.status == "OK"`, data)
	if err != nil {
		t.Fatalf("EvaluateBool error: %v", err)
	}
	if got {
		t.Error("expected false")
	}
}

func TestExprEngine_EvaluateNumeric(t *testing.T) {
	e := expression.NewExprEngine()
	data := map[string]any{"a": 10, "b": 20}
	got, err := e.Evaluate(`data.a + data.b`, data)
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if got != 30 {
		t.Errorf("got %v, want 30", got)
	}
}

func TestExprEngine_CacheHit(t *testing.T) {
	e := expression.NewExprEngine()
	data := map[string]any{"x": "a"}
	for i := range 3 {
		got, err := e.EvaluateString(`data.x`, data)
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
		if got != "a" {
			t.Errorf("iteration %d: got %q", i, got)
		}
	}
}

func TestExprEngine_ErrorInvalidExpr(t *testing.T) {
	e := expression.NewExprEngine()
	_, err := e.Evaluate(`!!!`, map[string]any{"x": "a"})
	if err == nil {
		t.Error("expected error for invalid expression")
	}
}

func TestExprEngine_EvaluateBool_TypeMismatch(t *testing.T) {
	e := expression.NewExprEngine()
	_, err := e.EvaluateBool(`"hello"`, map[string]any{})
	if err == nil {
		t.Error("expected error when result is not bool")
	}
}

func TestExprEngine_EvaluateString_TypeMismatch(t *testing.T) {
	e := expression.NewExprEngine()
	_, err := e.EvaluateString(`true`, map[string]any{})
	if err == nil {
		t.Error("expected error when result is not string")
	}
}

func TestExprEngine_MapData(t *testing.T) {
	e := expression.NewExprEngine()
	data := map[string]any{
		"payload": `{"host":"server1"}`,
		"input":   "BESZEL",
	}
	got, err := e.EvaluateString(`data.input + ": " + data.payload`, data)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := `BESZEL: {"host":"server1"}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
