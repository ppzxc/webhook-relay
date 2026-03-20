package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"webhook-relay/internal/domain"
)

func TestAlertStatus_IsValid(t *testing.T) {
	tests := []struct {
		name  string
		input domain.AlertStatus
		want  bool
	}{
		{"pending", domain.AlertStatusPending, true},
		{"delivered", domain.AlertStatusDelivered, true},
		{"failed", domain.AlertStatusFailed, true},
		{"unknown", domain.AlertStatus("UNKNOWN"), false},
		{"empty", domain.AlertStatus(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.input.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from domain.AlertStatus
		to   domain.AlertStatus
		want bool
	}{
		{domain.AlertStatusPending, domain.AlertStatusDelivered, true},
		{domain.AlertStatusPending, domain.AlertStatusFailed, true},
		{domain.AlertStatusFailed, domain.AlertStatusPending, true},
		{domain.AlertStatusDelivered, domain.AlertStatusPending, false},
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

func TestAlert_JSON_RoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	a := domain.Alert{
		ID:        "01J...",
		Version:   1,
		Source:    domain.SourceTypeBeszel,
		Payload:   domain.RawPayload(`{"host":"server1"}`),
		CreatedAt: now,
		Status:    domain.AlertStatusPending,
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got domain.Alert
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != a.ID || got.Status != a.Status || string(got.Payload) != string(a.Payload) {
		t.Errorf("round-trip mismatch")
	}
}
