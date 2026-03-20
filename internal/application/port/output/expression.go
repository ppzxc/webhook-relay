package output

// ExpressionEngine evaluates expressions against a data map.
type ExpressionEngine interface {
	Evaluate(expression string, data map[string]any) (any, error)
	EvaluateBool(expression string, data map[string]any) (bool, error)
	EvaluateString(expression string, data map[string]any) (string, error)
	Type() string
}

// ExpressionEngineRegistry manages available expression engines.
type ExpressionEngineRegistry interface {
	Get(engineType string) (ExpressionEngine, error)
	Default() ExpressionEngine
}
