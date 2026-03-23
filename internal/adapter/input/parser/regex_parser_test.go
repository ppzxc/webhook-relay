package parser_test

import (
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestRegexParser_Type(t *testing.T) {
	p, err := parser.NewRegexParser(`(?P<host>\w+)`)
	if err != nil {
		t.Fatalf("NewRegexParser error: %v", err)
	}
	if got := p.Type(); got != "REGEX" {
		t.Errorf("Type() = %q, want %q", got, "REGEX")
	}
}

func TestRegexParser_InvalidPattern(t *testing.T) {
	_, err := parser.NewRegexParser(`[invalid`)
	if err == nil {
		t.Fatal("expected error for invalid pattern")
	}
}

func TestRegexParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		body    []byte
		wantErr bool
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name:    "match with named groups",
			pattern: `(?P<host>\w+):(?P<port>\d+)`,
			body:    []byte("server1:8080"),
			check: func(t *testing.T, result map[string]any) {
				if result["host"] != "server1" {
					t.Errorf("host = %v, want server1", result["host"])
				}
				if result["port"] != "8080" {
					t.Errorf("port = %v, want 8080", result["port"])
				}
			},
		},
		{
			name:    "no match",
			pattern: `(?P<host>\d+)`,
			body:    []byte("no-digits-here"),
			wantErr: true,
		},
		{
			name:    "no named groups",
			pattern: `(\w+):(\d+)`,
			body:    []byte("server1:8080"),
			wantErr: true,
		},
		{
			name:    "empty body",
			pattern: `(?P<host>\w+)`,
			body:    []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parser.NewRegexParser(tt.pattern)
			if err != nil {
				t.Fatalf("NewRegexParser error: %v", err)
			}
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
