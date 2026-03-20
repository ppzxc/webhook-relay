package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"relaybox/internal/config"
)

const testYAML = `
server:
  port: 9090
log:
  level: debug
  format: text
inputs:
  - id: beszel
    type: BESZEL
    secret: test-secret
outputs:
  - id: ops-webhook
    type: WEBHOOK
    url: https://hooks.example.com/test
    template:
      text: 'input + ": " + payload'
    retryCount: 3
    retryDelayMs: 500
rules:
  - inputId: beszel
    outputIds:
      - ops-webhook
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
  workerCount: 1
`

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, testYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if len(cfg.Inputs) != 1 || cfg.Inputs[0].ID != "beszel" {
		t.Errorf("inputs = %+v", cfg.Inputs)
	}
	if len(cfg.Rules) != 1 {
		t.Errorf("rules = %+v", cfg.Rules)
	}
	if cfg.Outputs[0].Template["text"] != `input + ": " + payload` {
		t.Errorf("template = %+v", cfg.Outputs[0].Template)
	}
}

func TestLoad_EmptyInputID(t *testing.T) {
	yaml := `
inputs:
  - id: ""
    type: BESZEL
    secret: s
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for empty input ID")
	}
}

func TestLoad_DuplicateInputID(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    type: BESZEL
    secret: s1
  - id: beszel
    type: BESZEL
    secret: s2
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for duplicate input ID")
	}
}

func TestLoad_RuleReferencesUnknownOutput(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    type: BESZEL
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    url: https://example.com
rules:
  - inputId: beszel
    outputIds:
      - nonexistent-output
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for rule referencing unknown output")
	}
}

func TestInMemoryRuleConfigReader(t *testing.T) {
	cfg, _ := config.Load(writeConfig(t, testYAML))
	reader := config.NewInMemoryRuleConfigReader(cfg)

	rule, outputs, err := reader.GetRule(nil, "BESZEL")
	if err != nil {
		t.Fatalf("GetRule error: %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("got %d outputs, want 1", len(outputs))
	}
	if rule.InputID != "beszel" {
		t.Errorf("rule.InputID = %q, want beszel", rule.InputID)
	}
}

func TestLoad_WithExpressionConfig(t *testing.T) {
	yaml := `
expression:
  defaultEngine: expr
inputs:
  - id: beszel
    type: BESZEL
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    url: https://example.com
rules:
  - inputId: beszel
    engine: expr
    filter: 'status == "CRITICAL"'
    mapping:
      severity: '"HIGH"'
    routing:
      - condition: 'severity == "HIGH"'
        outputIds: [ch1]
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
`
	cfg, err := config.Load(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Expression.DefaultEngine != "expr" {
		t.Errorf("defaultEngine = %q, want expr", cfg.Expression.DefaultEngine)
	}
	if cfg.Rules[0].Engine != "expr" {
		t.Errorf("rule engine = %q, want expr", cfg.Rules[0].Engine)
	}
	if cfg.Rules[0].Filter != `status == "CRITICAL"` {
		t.Errorf("filter = %q", cfg.Rules[0].Filter)
	}
	if len(cfg.Rules[0].Routing) != 1 {
		t.Errorf("routing len = %d, want 1", len(cfg.Rules[0].Routing))
	}

	reader := config.NewInMemoryRuleConfigReader(cfg)
	rule, outputs, err := reader.GetRule(nil, "BESZEL")
	if err != nil {
		t.Fatalf("GetRule error: %v", err)
	}
	if rule.Engine != "expr" {
		t.Errorf("rule.Engine = %q, want expr", rule.Engine)
	}
	if rule.Filter != `status == "CRITICAL"` {
		t.Errorf("rule.Filter = %q", rule.Filter)
	}
	if len(rule.Routing) != 1 {
		t.Errorf("rule.Routing len = %d", len(rule.Routing))
	}
	if len(outputs) != 1 {
		t.Errorf("outputs len = %d, want 1", len(outputs))
	}
}
