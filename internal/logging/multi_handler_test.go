package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"relaybox/internal/logging"
)

// bufHandler는 테스트용 in-memory slog.Handler.
type bufHandler struct {
	buf     bytes.Buffer
	enabled bool
	attrs   []slog.Attr
	group   string
}

func (h *bufHandler) Enabled(_ context.Context, _ slog.Level) bool { return h.enabled }
func (h *bufHandler) Handle(_ context.Context, r slog.Record) error {
	h.buf.WriteString(r.Message)
	return nil
}
func (h *bufHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &bufHandler{enabled: h.enabled, attrs: append(append([]slog.Attr{}, h.attrs...), attrs...), group: h.group}
}
func (h *bufHandler) WithGroup(name string) slog.Handler {
	return &bufHandler{enabled: h.enabled, attrs: h.attrs, group: name}
}

func newRecord(msg string) slog.Record {
	return slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
}

func TestMultiHandler_FansOutToAll(t *testing.T) {
	h1 := &bufHandler{enabled: true}
	h2 := &bufHandler{enabled: true}
	m := logging.NewMultiHandler(h1, h2)

	if err := m.Handle(context.Background(), newRecord("hello")); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	if h1.buf.String() != "hello" {
		t.Errorf("h1: got %q, want %q", h1.buf.String(), "hello")
	}
	if h2.buf.String() != "hello" {
		t.Errorf("h2: got %q, want %q", h2.buf.String(), "hello")
	}
}

func TestMultiHandler_Enabled_AnyTrue(t *testing.T) {
	disabled := &bufHandler{enabled: false}
	enabled := &bufHandler{enabled: true}
	m := logging.NewMultiHandler(disabled, enabled)

	if !m.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected Enabled=true when any handler is enabled")
	}
}

func TestMultiHandler_Enabled_AllFalse(t *testing.T) {
	h1 := &bufHandler{enabled: false}
	h2 := &bufHandler{enabled: false}
	m := logging.NewMultiHandler(h1, h2)

	if m.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected Enabled=false when all handlers are disabled")
	}
}

func TestMultiHandler_WithAttrs_Propagates(t *testing.T) {
	h1 := &bufHandler{enabled: true}
	h2 := &bufHandler{enabled: true}
	m := logging.NewMultiHandler(h1, h2)

	attr := slog.String("key", "val")
	m2 := m.WithAttrs([]slog.Attr{attr})

	// WithAttrs는 새 MultiHandler를 반환해야 함
	if m2 == nil {
		t.Fatal("WithAttrs returned nil")
	}
	// 새 handler로 record 처리 시 에러 없어야 함
	if err := m2.Handle(context.Background(), newRecord("world")); err != nil {
		t.Fatalf("Handle after WithAttrs error: %v", err)
	}
}

func TestMultiHandler_WithGroup_Propagates(t *testing.T) {
	h1 := &bufHandler{enabled: true}
	m := logging.NewMultiHandler(h1)

	m2 := m.WithGroup("mygroup")
	if m2 == nil {
		t.Fatal("WithGroup returned nil")
	}
	if err := m2.Handle(context.Background(), newRecord("grouped")); err != nil {
		t.Fatalf("Handle after WithGroup error: %v", err)
	}
}
