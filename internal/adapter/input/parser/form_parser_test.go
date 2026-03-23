package parser_test

import (
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestFormParser_Type(t *testing.T) {
	p := parser.NewFormParser()
	if got := p.Type(); got != "FORM" {
		t.Errorf("Type() = %q, want %q", got, "FORM")
	}
}

func TestFormParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name: "single values",
			body: []byte("name=alice&age=30"),
			check: func(t *testing.T, result map[string]any) {
				if result["name"] != "alice" {
					t.Errorf("name = %v, want alice", result["name"])
				}
				if result["age"] != "30" {
					t.Errorf("age = %v, want 30", result["age"])
				}
			},
		},
		{
			name: "multi-values",
			body: []byte("color=red&color=blue&color=green"),
			check: func(t *testing.T, result map[string]any) {
				colors, ok := result["color"].([]string)
				if !ok {
					t.Fatalf("color should be []string, got %T", result["color"])
				}
				if len(colors) != 3 {
					t.Errorf("len(colors) = %d, want 3", len(colors))
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

	p := parser.NewFormParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse("application/x-www-form-urlencoded", tt.body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
