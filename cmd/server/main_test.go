package main

import (
	"testing"

	cfgpkg "relaybox/internal/config"
)

func TestNewRepository_Sqlite(t *testing.T) {
	repo, closer, err := newRepository(cfgpkg.StorageConfig{Type: "SQLITE", Path: ":memory:"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if err := closer.Close(); err != nil {
			t.Errorf("close error: %v", err)
		}
	}()
	if repo == nil {
		t.Error("expected non-nil repository")
	}
}

func TestNewRepository_CaseInsensitive(t *testing.T) {
	_, closer, err := newRepository(cfgpkg.StorageConfig{Type: "sqlite", Path: ":memory:"})
	if err != nil {
		t.Fatalf("sqlite lowercase should be accepted: %v", err)
	}
	closer.Close()
}

func TestNewRepository_UnsupportedType(t *testing.T) {
	_, _, err := newRepository(cfgpkg.StorageConfig{Type: "UNKNOWN", Path: ""})
	if err == nil {
		t.Fatal("expected error for unsupported storage type")
	}
}
