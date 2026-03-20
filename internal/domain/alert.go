package domain

import (
	"time"
)

type RawPayload []byte

func (r RawPayload) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return r, nil
}

func (r *RawPayload) UnmarshalJSON(b []byte) error {
	*r = make(RawPayload, len(b))
	copy(*r, b)
	return nil
}

type Alert struct {
	ID            string      `json:"id"`
	Version       int         `json:"version"`
	Source        SourceType  `json:"source"`
	Payload       RawPayload  `json:"payload"`
	CreatedAt     time.Time   `json:"createdAt"`
	Status        AlertStatus `json:"status"`
	RetryCount    int         `json:"retryCount"`
	LastAttemptAt *time.Time  `json:"lastAttemptAt,omitempty"`
}

