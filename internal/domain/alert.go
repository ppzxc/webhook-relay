package domain

import (
	"encoding/json"
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

func (a Alert) MarshalJSON() ([]byte, error) {
	type Alias struct {
		ID            string     `json:"id"`
		Version       int        `json:"version"`
		Source        string     `json:"source"`
		Payload       RawPayload `json:"payload"`
		CreatedAt     time.Time  `json:"createdAt"`
		Status        string     `json:"status"`
		RetryCount    int        `json:"retryCount"`
		LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty"`
	}
	return json.Marshal(Alias{
		ID: a.ID, Version: a.Version, Source: string(a.Source),
		Payload: a.Payload, CreatedAt: a.CreatedAt, Status: string(a.Status),
		RetryCount: a.RetryCount, LastAttemptAt: a.LastAttemptAt,
	})
}
