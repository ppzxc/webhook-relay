package parser_test

import (
	"errors"
	"testing"

	"relaybox/internal/adapter/input/parser"
)

func TestRegistry_GetExisting(t *testing.T) {
	reg := parser.NewInMemoryParserRegistry()
	reg.Register(parser.NewJSONParser())

	p, err := reg.Get("JSON")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if p.Type() != "JSON" {
		t.Errorf("Type() = %q, want %q", p.Type(), "JSON")
	}
}

func TestRegistry_GetMissing(t *testing.T) {
	reg := parser.NewInMemoryParserRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing parser")
	}
	if !errors.Is(err, parser.ErrParserNotFound) {
		t.Errorf("error = %v, want ErrParserNotFound", err)
	}
}

func TestRegistry_RegisterOverwrite(t *testing.T) {
	reg := parser.NewInMemoryParserRegistry()
	reg.Register(parser.NewJSONParser())
	reg.Register(parser.NewJSONParser()) // should not panic

	p, err := reg.Get("JSON")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if p.Type() != "JSON" {
		t.Errorf("Type() = %q, want %q", p.Type(), "JSON")
	}
}
