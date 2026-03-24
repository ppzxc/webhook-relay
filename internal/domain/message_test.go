package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"relaybox/internal/domain"
)

func TestMessageStatus_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		input domain.MessageStatus
		want  bool
	}{
		{"pending", domain.MessageStatusPending, true},
		{"delivered", domain.MessageStatusDelivered, true},
		{"failed", domain.MessageStatusFailed, true},
		{"unknown", domain.MessageStatus("UNKNOWN"), false},
		{"empty", domain.MessageStatus(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessageStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from domain.MessageStatus
		to   domain.MessageStatus
		want bool
	}{
		{domain.MessageStatusPending, domain.MessageStatusDelivered, true},
		{domain.MessageStatusPending, domain.MessageStatusFailed, true},
		{domain.MessageStatusFailed, domain.MessageStatusPending, true},
		{domain.MessageStatusDelivered, domain.MessageStatusPending, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRawPayload_MarshalJSON(t *testing.T) {
	payload := domain.RawPayload(`{"level":"critical"}`)
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var result domain.RawPayload
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if string(result) != string(payload) {
		t.Errorf("got %s, want %s", result, payload)
	}
}

func TestMessage_JSON_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Message{
		ID:        "01J...",
		Version:   1,
		Input:     "beszel",
		Payload:   domain.RawPayload(`{"host":"server1"}`),
		CreatedAt: now,
		Status:    domain.MessageStatusPending,
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got domain.Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != a.ID || got.Status != a.Status || string(got.Payload) != string(a.Payload) {
		t.Errorf("round-trip mismatch")
	}
}
