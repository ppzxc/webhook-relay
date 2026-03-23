package domain

// RouteCondition maps a boolean expression to a set of output IDs.
type RouteCondition struct {
	Condition string
	OutputIDs []string
}

// Rule describes how to process messages from a given input.
type Rule struct {
	Filter  string            // expression; empty means pass all
	Mapping map[string]string // key -> expression
	Routing []RouteCondition
	OutputIDs []string // simple mode: send to all listed outputs (no routing)
}

// RuleEntry pairs a rule with the outputs it routes to.
type RuleEntry struct {
	Rule    Rule
	Outputs []Output
}
