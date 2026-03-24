package main

import (
	"encoding/base64"
	"testing"
	"time"

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

func TestBuildRelayWorkerConfig_ValidDurations(t *testing.T) {
	wc := cfgpkg.WorkerConfig{
		DefaultRetryCount: 5,
		DefaultRetryDelay: "2s",
		PollBackoff:       "100ms",
	}
	cfg, err := buildRelayWorkerConfig(wc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultRetryDelay != 2*time.Second {
		t.Errorf("DefaultRetryDelay = %v, want 2s", cfg.DefaultRetryDelay)
	}
	if cfg.PollBackoff != 100*time.Millisecond {
		t.Errorf("PollBackoff = %v, want 100ms", cfg.PollBackoff)
	}
	if cfg.DefaultRetryCount != 5 {
		t.Errorf("DefaultRetryCount = %d, want 5", cfg.DefaultRetryCount)
	}
}

func TestBuildRelayWorkerConfig_InvalidRetryDelay(t *testing.T) {
	wc := cfgpkg.WorkerConfig{DefaultRetryDelay: "not-a-duration", PollBackoff: "500ms"}
	_, err := buildRelayWorkerConfig(wc)
	if err == nil {
		t.Fatal("expected error for invalid defaultRetryDelay")
	}
}

func TestBuildRelayWorkerConfig_InvalidPollBackoff(t *testing.T) {
	wc := cfgpkg.WorkerConfig{DefaultRetryDelay: "1s", PollBackoff: "bad"}
	_, err := buildRelayWorkerConfig(wc)
	if err == nil {
		t.Fatal("expected error for invalid pollBackoff")
	}
}

func TestGenerateSecret_DefaultLength(t *testing.T) {
	s, err := generateSecret(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 bytes → base64url without padding = ceil(32*4/3) = 43 chars
	if len(s) != 43 {
		t.Errorf("secret length = %d, want 43", len(s))
	}
}

func TestGenerateSecret_OutputIsBase64URL(t *testing.T) {
	s, err := generateSecret(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("secret is not valid base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded length = %d, want 32", len(decoded))
	}
}

func TestGenerateSecret_LengthTooShort(t *testing.T) {
	_, err := generateSecret(15)
	if err == nil {
		t.Fatal("expected error for length < 16")
	}
}

func TestGenerateSecret_UniqueEachCall(t *testing.T) {
	a, err := generateSecret(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := generateSecret(32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == b {
		t.Error("two calls produced the same secret")
	}
}
