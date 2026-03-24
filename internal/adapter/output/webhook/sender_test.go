package webhook_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relaybox/internal/adapter/output/webhook"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
	"relaybox/internal/testutil"
)

// compile-time interface check
var _ output.OutputSender = (*webhook.Sender)(nil)
var _ output.OutputRegistry = (*webhook.Registry)(nil)

func TestSender_Timeout(t *testing.T) {
	quit := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-quit:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(quit)
		srv.CloseClientConnections()
		srv.Close()
	})

	sender := webhook.NewSender()
	out := domain.Output{URL: srv.URL, TimeoutSec: 1}

	start := time.Now()
	err := sender.Send(context.Background(), out, []byte(`{}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("Send took too long (%v), timeout not applied", elapsed)
	}
}

func TestSender_Send(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := webhook.NewSender()
	out := domain.Output{URL: srv.URL}
	payload := []byte(`{"text":"BESZEL"}`)

	if err := sender.Send(context.Background(), out, payload); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if string(received) != `{"text":"BESZEL"}` {
		t.Errorf("body = %q", received)
	}
}

func TestSender_LogsInfoOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	h := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(orig)

	sender := webhook.NewSender()
	out := domain.Output{ID: "test-output", URL: srv.URL}

	if err := sender.Send(context.Background(), out, []byte(`{"test":true}`)); err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	var found bool
	for _, rec := range h.Records() {
		if rec.Level == slog.LevelInfo && rec.Msg == "webhook sent" {
			if _, ok := rec.Attrs["output"]; !ok {
				t.Error("log record missing key: output")
			}
			if _, ok := rec.Attrs["statusCode"]; !ok {
				t.Error("log record missing key: statusCode")
			}
			if _, ok := rec.Attrs["elapsed"]; !ok {
				t.Error("log record missing key: elapsed")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error(`expected INFO "webhook sent" log record not found`)
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := webhook.NewRegistry(map[domain.OutputType]output.OutputSender{
		domain.OutputTypeWebhook: webhook.NewSender(),
	})
	got, err := reg.Get(domain.OutputTypeWebhook)
	if err != nil || got == nil {
		t.Errorf("Get(WEBHOOK): err=%v", err)
	}
	_, err = reg.Get(domain.OutputTypeSlack)
	if err == nil {
		t.Error("expected error for unregistered type")
	}
}
