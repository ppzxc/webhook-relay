// Package testutil provides shared test helpers.
package testutil

import (
	"context"
	"log/slog"
	"sync"
)

// LogRecord holds a captured slog record.
type LogRecord struct {
	Level  slog.Level
	Msg    string
	Attrs  map[string]any
}

// CaptureHandler is a minimal slog.Handler that collects log records.
type CaptureHandler struct {
	mu      sync.Mutex
	records []LogRecord
}

func (h *CaptureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *CaptureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, LogRecord{Level: r.Level, Msg: r.Message, Attrs: attrs})
	h.mu.Unlock()
	return nil
}

func (h *CaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *CaptureHandler) WithGroup(_ string) slog.Handler      { return h }

// Records returns a copy of all captured records.
func (h *CaptureHandler) Records() []LogRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]LogRecord, len(h.records))
	copy(out, h.records)
	return out
}
