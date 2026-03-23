package parser_test

import (
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestXMLParser_Type(t *testing.T) {
	p := parser.NewXMLParser()
	if got := p.Type(); got != "XML" {
		t.Errorf("Type() = %q, want %q", got, "XML")
	}
}

func TestXMLParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantErr bool
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name: "simple XML",
			body: []byte(`<root><host>server1</host><port>8080</port></root>`),
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
			name: "nested elements last value wins",
			body: []byte(`<root><data><name>test</name></data></root>`),
			check: func(t *testing.T, result map[string]any) {
				if result["name"] != "test" {
					t.Errorf("name = %v, want test", result["name"])
				}
			},
		},
		{
			name:    "empty body",
			body:    []byte{},
			wantErr: true,
		},
		{
			name:    "no elements",
			body:    []byte(`<!-- just a comment -->`),
			wantErr: true,
		},
	}

	p := parser.NewXMLParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse("application/xml", tt.body)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
