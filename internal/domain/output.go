package domain

type Output struct {
	ID            string
	Type          OutputType
	Engine        string            // "CEL" or "EXPR"
	URL           string
	Template      map[string]string // key -> CEL/Expr expression
	Secret        string
	RetryCount    int
	RetryDelayMs  int
	TimeoutSec    int
	SkipTLSVerify bool
}
