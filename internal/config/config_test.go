package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relaybox/internal/config"
)

// testYAML uses the new structure: rules inside inputs, engine required on inputs/outputs.
const testYAML = `
server:
  port: 9090
log:
  level: DEBUG
  format: TEXT
inputs:
  - id: beszel
    secret: test-secret
    engine: CEL
    rules:
      - outputIds:
          - ops-webhook
outputs:
  - id: ops-webhook
    type: WEBHOOK
    engine: CEL
    url: https://hooks.example.com/test
    template:
      text: 'data.input + ": " + data.payload'
    retryCount: 3
    retryDelayMs: 500
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
	if cfg.Inputs[0].Engine != "CEL" {
		t.Errorf("input engine = %q, want CEL", cfg.Inputs[0].Engine)
	}
	if len(cfg.Inputs[0].Rules) != 1 {
		t.Errorf("input rules len = %d, want 1", len(cfg.Inputs[0].Rules))
	}
	if cfg.Outputs[0].Engine != "CEL" {
		t.Errorf("output engine = %q, want CEL", cfg.Outputs[0].Engine)
	}
	if cfg.Outputs[0].Template["text"] != `data.input + ": " + data.payload` {
		t.Errorf("template = %+v", cfg.Outputs[0].Template)
	}
}

func TestLoad_EmptyInputID(t *testing.T) {
	yaml := `
inputs:
  - id: ""
    engine: CEL
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
    engine: CEL
    secret: s1
  - id: beszel
    engine: CEL
    secret: s2
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for duplicate input ID")
	}
}

func TestLoad_InputEngineRequired(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing input engine")
	}
}

func TestLoad_OutputEngineRequired(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: CEL
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    url: https://example.com
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing output engine")
	}
}

func TestLoad_RuleReferencesUnknownOutput(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: CEL
    secret: s
    rules:
      - outputIds:
          - nonexistent-output
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for rule referencing unknown output")
	}
}

func TestLoad_MultipleRulesPerInput(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: CEL
    secret: s
    rules:
      - outputIds: [ch1]
      - filter: 'data.severity == "HIGH"'
        outputIds: [ch2]
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com/1
  - id: ch2
    type: WEBHOOK
    engine: EXPR
    url: https://example.com/2
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
	if len(cfg.Inputs[0].Rules) != 2 {
		t.Errorf("input rules len = %d, want 2", len(cfg.Inputs[0].Rules))
	}
	if cfg.Inputs[0].Rules[1].Filter != `data.severity == "HIGH"` {
		t.Errorf("rule[1].filter = %q", cfg.Inputs[0].Rules[1].Filter)
	}
}

func TestLoad_WithFilterMappingRouting(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: CEL
    secret: s
    rules:
      - filter: 'data.status == "CRITICAL"'
        mapping:
          severity: '"HIGH"'
        routing:
          - condition: 'data.severity == "HIGH"'
            outputIds: [ch1]
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com
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
	rule := cfg.Inputs[0].Rules[0]
	if rule.Filter != `data.status == "CRITICAL"` {
		t.Errorf("filter = %q", rule.Filter)
	}
	if rule.Mapping["severity"] != `"HIGH"` {
		t.Errorf("mapping[severity] = %q", rule.Mapping["severity"])
	}
	if len(rule.Routing) != 1 {
		t.Errorf("routing len = %d, want 1", len(rule.Routing))
	}
}

func TestLoad_InvalidInputEngine(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: INVALID
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid input engine")
	}
	if !strings.Contains(err.Error(), "unsupported engine") {
		t.Errorf("error should mention unsupported engine, got: %v", err)
	}
}

func TestLoad_InvalidOutputEngine(t *testing.T) {
	yaml := `
inputs:
  - id: beszel
    engine: CEL
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    engine: INVALID
    url: https://example.com
storage:
  type: SQLITE
  path: ./data/test.db
queue:
  type: FILE
  path: ./data/queue
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid output engine")
	}
	if !strings.Contains(err.Error(), "unsupported engine") {
		t.Errorf("error should mention unsupported engine, got: %v", err)
	}
}

func TestLoad_WorkerConfigDefaults(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, testYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Worker.DefaultRetryCount != 3 {
		t.Errorf("worker.defaultRetryCount = %d, want 3", cfg.Worker.DefaultRetryCount)
	}
	if cfg.Worker.DefaultRetryDelay != "1s" {
		t.Errorf("worker.defaultRetryDelay = %q, want \"1s\"", cfg.Worker.DefaultRetryDelay)
	}
	if cfg.Worker.PollBackoff != "500ms" {
		t.Errorf("worker.pollBackoff = %q, want \"500ms\"", cfg.Worker.PollBackoff)
	}
}

func TestLoad_WorkerConfigCustom(t *testing.T) {
	yaml := testYAML + `
worker:
  defaultRetryCount: 5
  defaultRetryDelay: "2s"
  pollBackoff: "1s"
`
	cfg, err := config.Load(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Worker.DefaultRetryCount != 5 {
		t.Errorf("worker.defaultRetryCount = %d, want 5", cfg.Worker.DefaultRetryCount)
	}
	if cfg.Worker.DefaultRetryDelay != "2s" {
		t.Errorf("worker.defaultRetryDelay = %q, want \"2s\"", cfg.Worker.DefaultRetryDelay)
	}
	if cfg.Worker.PollBackoff != "1s" {
		t.Errorf("worker.pollBackoff = %q, want \"1s\"", cfg.Worker.PollBackoff)
	}
}

func TestLoad_RotationConfigDefaults(t *testing.T) {
	cfg, err := config.Load(writeConfig(t, testYAML))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Storage.Rotation.Enabled {
		t.Error("rotation.enabled should default to false")
	}
	if cfg.Storage.Rotation.Retention != "720h" {
		t.Errorf("rotation.retention = %q, want \"720h\"", cfg.Storage.Rotation.Retention)
	}
	if cfg.Storage.Rotation.Interval != "1h" {
		t.Errorf("rotation.interval = %q, want \"1h\"", cfg.Storage.Rotation.Interval)
	}
}

const rotationBaseYAML = `
inputs:
  - id: beszel
    engine: CEL
    secret: s
outputs:
  - id: ch1
    type: WEBHOOK
    engine: CEL
    url: https://example.com
queue:
  type: FILE
  path: ./data/queue
`

func TestLoad_RotationConfigCustom(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    retention: "168h"
    interval: "30m"
    statuses: ["DELIVERED"]
`
	cfg, err := config.Load(writeConfig(t, yaml))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !cfg.Storage.Rotation.Enabled {
		t.Error("rotation.enabled should be true")
	}
	if cfg.Storage.Rotation.Retention != "168h" {
		t.Errorf("rotation.retention = %q, want \"168h\"", cfg.Storage.Rotation.Retention)
	}
	if len(cfg.Storage.Rotation.Statuses) != 1 || cfg.Storage.Rotation.Statuses[0] != "DELIVERED" {
		t.Errorf("rotation.statuses = %v, want [DELIVERED]", cfg.Storage.Rotation.Statuses)
	}
}

func TestLoad_RotationConfig_InvalidRetention(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    retention: "not-a-duration"
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid rotation.retention duration")
	}
	if !strings.Contains(err.Error(), "rotation.retention") {
		t.Errorf("error should mention rotation.retention, got: %v", err)
	}
}

func TestLoad_RotationConfig_InvalidInterval(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    interval: "bad"
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid rotation.interval duration")
	}
	if !strings.Contains(err.Error(), "rotation.interval") {
		t.Errorf("error should mention rotation.interval, got: %v", err)
	}
}

func TestLoad_RotationConfig_InvalidStatus(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    statuses: ["INVALID_STATUS"]
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for invalid rotation status")
	}
	if !strings.Contains(err.Error(), "rotation.statuses") {
		t.Errorf("error should mention rotation.statuses, got: %v", err)
	}
}

func TestLoad_RotationConfig_ZeroRetention(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    retention: "0s"
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for zero rotation.retention")
	}
	if !strings.Contains(err.Error(), "rotation.retention") {
		t.Errorf("error should mention rotation.retention, got: %v", err)
	}
}

func TestLoad_RotationConfig_ZeroInterval(t *testing.T) {
	yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  rotation:
    enabled: true
    interval: "0s"
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for zero rotation.interval")
	}
	if !strings.Contains(err.Error(), "rotation.interval") {
		t.Errorf("error should mention rotation.interval, got: %v", err)
	}
}

func TestValidateConfig_TableName(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		wantErr   bool
	}{
		{"default messages", "messages", false},
		{"with underscore", "relay_messages", false},
		{"with number suffix", "my_table_1", false},
		{"starts with underscore", "_table", false},
		{"starts with number", "1table", true},
		{"contains space", "my table", true},
		{"sql injection", "messages; DROP TABLE messages;--", true},
		{"too long 65 chars", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true}, // 65자
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := rotationBaseYAML + `
storage:
  type: SQLITE
  path: ./data/test.db
  tableName: ` + tc.tableName + `
`
			_, err := config.Load(writeConfig(t, yaml))
			if tc.wantErr && err == nil {
				t.Errorf("expected error for tableName=%q, got nil", tc.tableName)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for tableName=%q: %v", tc.tableName, err)
			}
		})
	}
}
