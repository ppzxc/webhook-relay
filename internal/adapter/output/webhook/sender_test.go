package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relaybox/internal/adapter/output/webhook"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
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
