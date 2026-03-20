package domain

type Output struct {
	ID            string
	Type          OutputType
	URL           string
	Template      string
	Secret        string
	RetryCount    int
	RetryDelayMs  int
	TimeoutSec    int
	SkipTLSVerify bool
}
