package expression

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEngine implements ExpressionEngine using google/cel-go.
type CELEngine struct {
	cache sync.Map // expression string -> *cel.Ast
}

// NewCELEngine creates a new CEL expression engine.
func NewCELEngine() *CELEngine {
	return &CELEngine{}
}

func (e *CELEngine) Type() string { return "cel" }

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

type celCached struct {
	env *cel.Env
	ast *cel.Ast
	prg cel.Program
}

func (e *CELEngine) eval(expression string, data map[string]any) (ref.Val, error) {
	cached, err := e.getOrCompile(expression, data)
	if err != nil {
		return nil, err
	}
	out, _, err := cached.prg.Eval(data)
	if err != nil {
		return nil, fmt.Errorf("cel eval: %w", err)
	}
	return out, nil
}

func (e *CELEngine) getOrCompile(expression string, data map[string]any) (*celCached, error) {
	if v, ok := e.cache.Load(expression); ok {
		return v.(*celCached), nil
	}

	// Build variable declarations from data keys
	opts := make([]cel.EnvOption, 0, len(data))
	for k, v := range data {
		opts = append(opts, cel.Variable(k, celTypeFromGo(v)))
	}

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("cel env: %w", err)
	}
	ast, iss := env.Parse(expression)
	if iss.Err() != nil {
		return nil, fmt.Errorf("cel parse: %w", iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel program: %w", err)
	}
	c := &celCached{env: env, ast: ast, prg: prg}
	e.cache.Store(expression, c)
	return c, nil
}

func celTypeFromGo(v any) *cel.Type {
	switch v.(type) {
	case bool:
		return cel.BoolType
	case int, int32, int64:
		return cel.IntType
	case float32, float64:
		return cel.DoubleType
	case string:
		return cel.StringType
	case []any:
		return cel.ListType(cel.DynType)
	case map[string]any:
		return cel.MapType(cel.StringType, cel.DynType)
	default:
		return cel.DynType
	}
}
