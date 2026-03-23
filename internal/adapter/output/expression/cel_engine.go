package expression

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEngine implements ExpressionEngine using google/cel-go.
// A single cel.Env is built at construction time with a "data" variable of type map(string, dyn).
// All expressions access input data via the "data" prefix (e.g., data.id, data["key"]).
// This avoids cache collisions caused by different runtime data schemas sharing the same expression key.
type CELEngine struct {
	env   *cel.Env  // built once at construction
	cache sync.Map  // expression string -> cel.Program
}

// NewCELEngine creates a new CEL expression engine.
// Returns an error if the CEL environment cannot be created.
func NewCELEngine() (*CELEngine, error) {
	env, err := cel.NewEnv(
		cel.Variable("data", cel.MapType(cel.StringType, cel.DynType)),
	)
	if err != nil {
		return nil, fmt.Errorf("create CEL env: %w", err)
	}
	return &CELEngine{env: env}, nil
}

func (e *CELEngine) Type() string { return "CEL" }

func (e *CELEngine) Evaluate(expression string, data map[string]any) (any, error) {
	val, err := e.eval(expression, data)
	if err != nil {
		return nil, err
	}
	return val.Value(), nil
}

func (e *CELEngine) EvaluateBool(expression string, data map[string]any) (bool, error) {
	val, err := e.eval(expression, data)
	if err != nil {
		return false, err
	}
	if val.Type() != types.BoolType {
		return false, fmt.Errorf("cel: expected bool, got %s", val.Type())
	}
	return val.Value().(bool), nil
}

func (e *CELEngine) EvaluateString(expression string, data map[string]any) (string, error) {
	val, err := e.eval(expression, data)
	if err != nil {
		return "", err
	}
	if val.Type() != types.StringType {
		return "", fmt.Errorf("cel: expected string, got %s", val.Type())
	}
	return val.Value().(string), nil
}

func (e *CELEngine) eval(expression string, data map[string]any) (ref.Val, error) {
	prog, err := e.getOrCompile(expression)
	if err != nil {
		return nil, err
	}
	out, _, err := prog.Eval(map[string]any{"data": data})
	if err != nil {
		return nil, fmt.Errorf("cel eval: %w", err)
	}
	return out, nil
}

func (e *CELEngine) getOrCompile(expression string) (cel.Program, error) {
	if v, ok := e.cache.Load(expression); ok {
		return v.(cel.Program), nil
	}
	ast, iss := e.env.Compile(expression)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("cel compile %q: %w", expression, iss.Err())
	}
	prog, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel program %q: %w", expression, err)
	}
	actual, _ := e.cache.LoadOrStore(expression, prog)
	return actual.(cel.Program), nil
}
