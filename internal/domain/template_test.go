package domain_test

import (
	"testing"
	"time"

	"webhook-relay/internal/domain"
)

func TestRenderTemplate(t *testing.T) {
	alert := domain.Alert{
		ID:        "abc123",
		Source:    domain.SourceTypeBeszel,
		Payload:   domain.RawPayload(`{"host":"server1"}`),
		CreatedAt: time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC),
		Status:    domain.AlertStatusPending,
	}

	tests := []struct {
		name    string
		tmpl    string
		want    string
		wantErr bool
	}{
		{
			name: "source and id",
			tmpl: `{"text":"{{ .Source }}: {{ .ID }}"}`,
			want: `{"text":"BESZEL: abc123"}`,
		},
		{
			name:    "invalid syntax",
			tmpl:    `{{ .Source`,
			wantErr: true,
		},
		{
			name: "payload field",
			tmpl: `{{ .Payload }}`,
			want: `{"host":"server1"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.RenderTemplate(tt.tmpl, alert)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	if err := domain.ValidateTemplate(`{{ .Source }}`); err != nil {
		t.Errorf("valid template failed: %v", err)
	}
	if err := domain.ValidateTemplate(`{{ .Source`); err == nil {
		t.Error("invalid template should return error")
	}
}
