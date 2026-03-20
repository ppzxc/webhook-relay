package webhook_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"webhook-relay/internal/adapter/output/webhook"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.AlertSender = (*webhook.Sender)(nil)
var _ output.SenderRegistry = (*webhook.Registry)(nil)

func TestSender_Send(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := webhook.NewSender()
	channel := domain.Channel{URL: srv.URL, Template: `{"text":"{{ .Source }}"}`}
	alert := domain.Alert{ID: "a1", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Version: 1}

	if err := sender.Send(context.Background(), channel, alert); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if string(received) != `{"text":"BESZEL"}` {
		t.Errorf("body = %q", received)
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := webhook.NewRegistry(map[domain.ChannelType]output.AlertSender{
		domain.ChannelTypeWebhook: webhook.NewSender(),
	})
	got, err := reg.Get(domain.ChannelTypeWebhook)
	if err != nil || got == nil {
		t.Errorf("Get(WEBHOOK): err=%v", err)
	}
	_, err = reg.Get(domain.ChannelTypeSlack)
	if err == nil {
		t.Error("expected error for unregistered type")
	}
}
