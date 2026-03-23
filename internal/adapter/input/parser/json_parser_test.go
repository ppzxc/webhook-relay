package parser_test

import (
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestJSONParser_Type(t *testing.T) {
	p := parser.NewJSONParser()
	if got := p.Type(); got != "JSON" {
		t.Errorf("Type() = %q, want %q", got, "JSON")
	}
}

func TestJSONParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		wantKey string
		wantVal any
	}{
		{
			name:    "valid JSON object",
			body:    []byte(`{"host":"server1","port":8080}`),
			wantKey: "host",
			wantVal: "server1",
		},
		{
			name:    "nested JSON",
			body:    []byte(`{"data":{"nested":true}}`),
			wantKey: "data",
		},
		{
			name:    "invalid JSON",
			body:    []byte(`{invalid`),
			wantErr: true,
		},
		{
			name:    "empty body",
			body:    []byte{},
			wantErr: true,
		},
		{
			name:    "nil body",
			body:    nil,
			wantErr: true,
		},
	}

	p := parser.NewJSONParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse("application/json", tt.body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantVal != nil {
				if got, ok := result[tt.wantKey]; !ok || got != tt.wantVal {
					t.Errorf("result[%q] = %v, want %v", tt.wantKey, got, tt.wantVal)
				}
			}
			if _, ok := result[tt.wantKey]; !ok {
				t.Errorf("expected key %q in result", tt.wantKey)
			}
		})
	}
}
