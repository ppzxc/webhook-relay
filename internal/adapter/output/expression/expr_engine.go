package expression

import (
	"fmt"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// ExprEngine implements ExpressionEngine using expr-lang/expr.
// All expressions access input data via the "data" prefix (e.g., data.id, data["key"]).
type ExprEngine struct {
	cache sync.Map // expression string -> *vm.Program
}

// NewExprEngine creates a new Expr expression engine.
func NewExprEngine() *ExprEngine {
	return &ExprEngine{}
}

func (e *ExprEngine) Type() string { return "EXPR" }

func (e *ExprEngine) Evaluate(expression string, data map[string]any) (any, error) {
	prg, err := e.getOrCompile(expression)
	if err != nil {
		return nil, err
	}
	out, err := expr.Run(prg, map[string]any{"data": data})
	if err != nil {
		return nil, fmt.Errorf("expr eval: %w", err)
	}
	return out, nil
}

func (e *ExprEngine) EvaluateBool(expression string, data map[string]any) (bool, error) {
	val, err := e.Evaluate(expression, data)
	if err != nil {
		return false, err
	}
	b, ok := val.(bool)
	if !ok {
		return false, fmt.Errorf("expr: expected bool, got %T", val)
	}
	return b, nil
}

func (e *ExprEngine) EvaluateString(expression string, data map[string]any) (string, error) {
	val, err := e.Evaluate(expression, data)
	if err != nil {
		return "", err
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("expr: expected string, got %T", val)
	}
	return s, nil
}

func (e *ExprEngine) getOrCompile(expression string) (*vm.Program, error) {
	if v, ok := e.cache.Load(expression); ok {
		return v.(*vm.Program), nil
	}
	prg, err := expr.Compile(expression, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("expr compile: %w", err)
	}
	actual, _ := e.cache.LoadOrStore(expression, prg)
	return actual.(*vm.Program), nil
}
