package main

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"warn", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},     // 빈 문자열 → INFO
		{"UNKNOWN", slog.LevelInfo}, // 알 수 없는 값 → INFO
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		specific string
		fallback string
		want     string
	}{
		{"TEXT", "JSON", "TEXT"},   // specific 우선
		{"JSON", "TEXT", "JSON"},
		{"", "JSON", "JSON"},       // 빈 문자열이면 fallback
		{"", "TEXT", "TEXT"},
		{"", "", ""},
		{"text", "JSON", "TEXT"},   // 대문자 정규화
	}
	for _, tt := range tests {
		t.Run(tt.specific+"/"+tt.fallback, func(t *testing.T) {
			got := resolveFormat(tt.specific, tt.fallback)
			if got != tt.want {
				t.Errorf("resolveFormat(%q, %q) = %q, want %q", tt.specific, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestSetupLogger_StdoutOnly(t *testing.T) {
	cfg := &cfgpkg.Config{
		Log: cfgpkg.LogConfig{
			Level:  "INFO",
			Format: "JSON",
			Stdout: cfgpkg.LogStdoutConfig{Enabled: true, Format: "JSON"},
			File:   cfgpkg.LogFileConfig{Enabled: false},
		},
	}
	// setupLogger가 패닉 없이 실행되는지 확인
	setupLogger(cfg)
	slog.Info("test stdout only")
	// slog.Default가 교체되었으면 성공
}

func TestSetupLogger_FileOnly(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	cfg := &cfgpkg.Config{
		Log: cfgpkg.LogConfig{
			Level:  "INFO",
			Format: "JSON",
			Stdout: cfgpkg.LogStdoutConfig{Enabled: false},
			File: cfgpkg.LogFileConfig{
				Enabled:    true,
				Format:     "JSON",
				Path:       logPath,
				MaxSizeMB:  100,
				MaxBackups: 3,
				MaxAgeDays: 30,
				Compress:   false,
			},
		},
	}
	setupLogger(cfg)
	slog.Info("file only test", "key", "value")

	// 파일이 생성되었는지 확인
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatalf("log file not created at %s", logPath)
	}
}

func TestSetupLogger_Dual(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "dual.log")

	cfg := &cfgpkg.Config{
		Log: cfgpkg.LogConfig{
			Level:  "INFO",
			Format: "JSON",
			Stdout: cfgpkg.LogStdoutConfig{Enabled: true, Format: "JSON"},
			File: cfgpkg.LogFileConfig{
				Enabled:    true,
				Format:     "JSON",
				Path:       logPath,
				MaxSizeMB:  100,
				MaxBackups: 3,
				MaxAgeDays: 30,
				Compress:   false,
			},
		},
	}
	setupLogger(cfg)
	slog.Info("dual test message", "env", "test")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Contains(content, []byte("dual test message")) {
		t.Errorf("log file does not contain expected message, got: %s", content)
	}
}

func TestSetupLogger_MaxSizeMB_Zero_NoError(t *testing.T) {
	dir := t.TempDir()
	cfg := &cfgpkg.Config{
		Log: cfgpkg.LogConfig{
			Level:  "INFO",
			Format: "JSON",
			Stdout: cfgpkg.LogStdoutConfig{Enabled: false},
			File: cfgpkg.LogFileConfig{
				Enabled:   true,
				Format:    "JSON",
				Path:      filepath.Join(dir, "unlimited.log"),
				MaxSizeMB: 0, // 무제한
			},
		},
	}
	// 패닉 없이 실행되어야 함
	setupLogger(cfg)
	slog.Info("unlimited size test")
}

func TestSetupLogger_LegacyFormatFallback(t *testing.T) {
	// stdout.format이 비어있으면 log.format을 사용
	dir := t.TempDir()
	logPath := filepath.Join(dir, "legacy.log")
	cfg := &cfgpkg.Config{
		Log: cfgpkg.LogConfig{
			Level:  "INFO",
			Format: "JSON", // fallback
			Stdout: cfgpkg.LogStdoutConfig{Enabled: false},
			File: cfgpkg.LogFileConfig{
				Enabled:   true,
				Format:    "", // 비어있으면 log.format 상속 → "JSON"
				Path:      logPath,
				MaxSizeMB: 100,
			},
		},
	}
	setupLogger(cfg)
	slog.Info("legacy format test")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	// JSON 형식이면 { 로 시작
	if !strings.Contains(string(content), `"msg"`) {
		t.Errorf("expected JSON format, got: %s", content)
	}
}
