package service

import "time"

// RelayWorkerConfig holds tunable parameters for RelayWorker.
// Zero values are replaced with defaults by withDefaults().
type RelayWorkerConfig struct {
	DefaultRetryCount   int
	DefaultRetryDelayMs int
	PollBackoff         time.Duration
}

// DefaultRelayWorkerConfig returns a config with the original hard-coded defaults.
func DefaultRelayWorkerConfig() RelayWorkerConfig {
	return RelayWorkerConfig{
		DefaultRetryCount:   3,
		DefaultRetryDelayMs: 1000,
		PollBackoff:         500 * time.Millisecond,
	}
}

// withDefaults replaces zero values with the original hard-coded defaults.
func (c RelayWorkerConfig) withDefaults() RelayWorkerConfig {
	d := DefaultRelayWorkerConfig()
	if c.DefaultRetryCount <= 0 {
		c.DefaultRetryCount = d.DefaultRetryCount
	}
	if c.DefaultRetryDelayMs <= 0 {
		c.DefaultRetryDelayMs = d.DefaultRetryDelayMs
	}
	if c.PollBackoff <= 0 {
		c.PollBackoff = d.PollBackoff
	}
	return c
}
