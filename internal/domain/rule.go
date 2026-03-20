package domain

// RouteCondition maps a boolean expression to a set of output IDs.
type RouteCondition struct {
	Condition string
	OutputIDs []string
}

// Rule describes how to process messages from a given input.
type Rule struct {
	InputID string
	Engine  string            // "cel" or "expr"; empty means use default
	Filter  string            // expression; empty means pass all
	Mapping map[string]string // key -> expression
	Routing []RouteCondition
}
