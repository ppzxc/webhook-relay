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

type Message struct {
	ID            string         `json:"id"`
	Version       int            `json:"version"`
	Input         InputType      `json:"input"`
	Payload       RawPayload     `json:"payload"`
	// ParsedData is populated at parse time from the raw Payload.
	// It is intentionally not persisted to SQLite — only stored in the file queue via JSON.
	// Available to the relay worker for expression evaluation.
	ParsedData map[string]any `json:"parsedData,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	Status        MessageStatus  `json:"status"`
	RetryCount    int            `json:"retryCount"`
	LastAttemptAt *time.Time     `json:"lastAttemptAt,omitempty"`
}
