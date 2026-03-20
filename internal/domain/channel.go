package domain

type Channel struct {
	ID            string
	Type          ChannelType
	URL           string
	Template      string
	Secret        string
	RetryCount    int
	RetryDelayMs  int
	SkipTLSVerify bool
}
