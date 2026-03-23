package parser_test

import (
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestLogfmtParser_Type(t *testing.T) {
	p := parser.NewLogfmtParser()
	if got := p.Type(); got != "LOGFMT" {
		t.Errorf("Type() = %q, want %q", got, "LOGFMT")
	}
}

func TestLogfmtParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name: "basic key=value pairs",
			body: []byte(`level=info msg=hello ts=12345`),
			check: func(t *testing.T, result map[string]any) {
				if result["level"] != "info" {
					t.Errorf("level = %v, want info", result["level"])
				}
				if result["msg"] != "hello" {
					t.Errorf("msg = %v, want hello", result["msg"])
				}
			},
		},
		{
			name: "quoted values",
			body: []byte(`key="value with spaces" other=simple`),
			check: func(t *testing.T, result map[string]any) {
				if result["key"] != "value with spaces" {
					t.Errorf("key = %v, want 'value with spaces'", result["key"])
				}
				if result["other"] != "simple" {
					t.Errorf("other = %v, want simple", result["other"])
				}
			},
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

	p := parser.NewLogfmtParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse("text/plain", tt.body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
