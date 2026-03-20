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
    template: '{"text":"{{ .Source }}"}'
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
}

func TestLoad_InvalidTemplate(t *testing.T) {
	yaml := `
server:
  port: 8080
outputs:
  - id: bad
    type: WEBHOOK
    url: https://example.com
    template: '{{ .Source'
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid template")
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
    template: '{}'
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

	outputs, err := reader.GetOutputs(nil, "BESZEL") // query by input type, not input ID
	if err != nil {
		t.Fatalf("GetOutputs error: %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("got %d outputs, want 1", len(outputs))
	}
}
