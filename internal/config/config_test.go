package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"webhook-relay/internal/config"
)

const testYAML = `
server:
  port: 9090
log:
  level: debug
  format: text
sources:
  - id: beszel
    type: BESZEL
    secret: test-secret
channels:
  - id: ops-webhook
    type: WEBHOOK
    url: https://hooks.example.com/test
    template: '{"text":"{{ .Source }}"}'
    retryCount: 3
    retryDelayMs: 500
routes:
  - sourceId: beszel
    channelIds:
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
	if len(cfg.Sources) != 1 || cfg.Sources[0].ID != "beszel" {
		t.Errorf("sources = %+v", cfg.Sources)
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("routes = %+v", cfg.Routes)
	}
}

func TestLoad_InvalidTemplate(t *testing.T) {
	yaml := `
server:
  port: 8080
channels:
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

func TestLoad_EmptySourceID(t *testing.T) {
	yaml := `
sources:
  - id: ""
    type: BESZEL
    secret: s
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for empty source ID")
	}
}

func TestLoad_DuplicateSourceID(t *testing.T) {
	yaml := `
sources:
  - id: beszel
    type: BESZEL
    secret: s1
  - id: beszel
    type: BESZEL
    secret: s2
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for duplicate source ID")
	}
}

func TestLoad_RouteReferencesUnknownChannel(t *testing.T) {
	yaml := `
sources:
  - id: beszel
    type: BESZEL
    secret: s
channels:
  - id: ch1
    type: WEBHOOK
    url: https://example.com
    template: '{}'
routes:
  - sourceId: beszel
    channelIds:
      - nonexistent-channel
`
	_, err := config.Load(writeConfig(t, yaml))
	if err == nil {
		t.Fatal("expected error for route referencing unknown channel")
	}
}

func TestInMemoryRouteConfigReader(t *testing.T) {
	cfg, _ := config.Load(writeConfig(t, testYAML))
	reader := config.NewInMemoryRouteConfigReader(cfg)

	channels, err := reader.GetChannels(nil, "BESZEL") // query by source type, not source ID
	if err != nil {
		t.Fatalf("GetChannels error: %v", err)
	}
	if len(channels) != 1 {
		t.Errorf("got %d channels, want 1", len(channels))
	}
}
