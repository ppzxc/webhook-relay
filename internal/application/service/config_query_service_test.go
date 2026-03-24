package service_test

import (
	"context"
	"errors"
	"testing"

	"relaybox/internal/application/service"
	cfgpkg "relaybox/internal/config"
	"relaybox/internal/domain"
)

func testConfig() *cfgpkg.Config {
	return &cfgpkg.Config{
		Inputs: []cfgpkg.InputConfig{
			{ID: "beszel", Engine: "CEL", Secret: "tok1"},
			{ID: "dozzle", Engine: "EXPR", Secret: "tok2"},
		},
		Outputs: []cfgpkg.OutputConfig{
			{ID: "wh1", Type: "WEBHOOK", Engine: "CEL", URL: "http://example.com/hook", RetryCount: 3, RetryDelayMs: 100},
			{ID: "wh2", Type: "SLACK", Engine: "CEL", URL: "http://hooks.slack.com/x", RetryCount: 1, RetryDelayMs: 0},
		},
	}
}

func TestConfigQueryService_ListInputs(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	inputs, err := svc.ListInputs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(inputs))
	}
	ids := map[string]bool{inputs[0].ID: true, inputs[1].ID: true}
	if !ids["beszel"] || !ids["dozzle"] {
		t.Errorf("unexpected input IDs: %v", inputs)
	}
}

func TestConfigQueryService_GetInput_Found(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	inp, err := svc.GetInput(context.Background(), "beszel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inp.ID != "beszel" {
		t.Errorf("expected id beszel, got %s", inp.ID)
	}
}

func TestConfigQueryService_GetInput_NotFound(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	_, err := svc.GetInput(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrInputNotFound) {
		t.Errorf("expected ErrInputNotFound, got %v", err)
	}
}

func TestConfigQueryService_ListOutputs(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	outputs, err := svc.ListOutputs(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outputs) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(outputs))
	}
}

func TestConfigQueryService_GetOutput_Found(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	out, err := svc.GetOutput(context.Background(), "wh1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "wh1" {
		t.Errorf("expected id wh1, got %s", out.ID)
	}
	if out.URL != "http://example.com/hook" {
		t.Errorf("expected URL http://example.com/hook, got %s", out.URL)
	}
	if out.RetryCount != 3 {
		t.Errorf("expected retryCount 3, got %d", out.RetryCount)
	}
	// Secret must NOT be exposed
}

func TestConfigQueryService_GetOutput_NotFound(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())
	_, err := svc.GetOutput(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, domain.ErrOutputNotFound) {
		t.Errorf("expected ErrOutputNotFound, got %v", err)
	}
}

func TestConfigQueryService_Update(t *testing.T) {
	svc := service.NewConfigQueryService(testConfig())

	// hot-reload: replace with single input/output
	newCfg := &cfgpkg.Config{
		Inputs:  []cfgpkg.InputConfig{{ID: "new-input", Engine: "CEL"}},
		Outputs: []cfgpkg.OutputConfig{{ID: "new-output", Type: "WEBHOOK", Engine: "CEL", URL: "http://new.example.com"}},
	}
	svc.Update(newCfg)

	inputs, _ := svc.ListInputs(context.Background())
	if len(inputs) != 1 || inputs[0].ID != "new-input" {
		t.Errorf("after update: expected [new-input], got %v", inputs)
	}
	outputs, _ := svc.ListOutputs(context.Background())
	if len(outputs) != 1 || outputs[0].ID != "new-output" {
		t.Errorf("after update: expected [new-output], got %v", outputs)
	}
	// old input/output must be gone
	_, err := svc.GetInput(context.Background(), "beszel")
	if !errors.Is(err, domain.ErrInputNotFound) {
		t.Errorf("old input should be gone after update, got err=%v", err)
	}
}
