package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

type mockAlertQueue struct {
	alerts []domain.Alert
	idx    int
}

func (m *mockAlertQueue) Enqueue(_ context.Context, _ domain.Alert) error { return nil }
func (m *mockAlertQueue) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	if m.idx >= len(m.alerts) {
		time.Sleep(10 * time.Millisecond)
		return domain.Alert{}, nil, nil, errors.New("empty")
	}
	a := m.alerts[m.idx]
	m.idx++
	return a, func() error { return nil }, func() error { return nil }, nil
}

type mockRouteReader struct{ channels []domain.Channel }

func (m *mockRouteReader) GetChannels(_ context.Context, _ string) ([]domain.Channel, error) {
	return m.channels, nil
}

type mockSender struct{ count atomic.Int32 }

func (m *mockSender) Send(_ context.Context, _ domain.Channel, _ domain.Alert) error {
	m.count.Add(1)
	return nil
}

type mockRegistry struct{ sender *mockSender }

func (m *mockRegistry) Get(_ domain.ChannelType) (output.AlertSender, error) {
	return m.sender, nil
}

func TestDeliveryWorker_UpdateDeliveryState_ErrorDoesNotBreakWorker(t *testing.T) {
	// UpdateDeliveryState가 에러를 반환해도 워커가 정상 동작(send 완료, 패닉 없음)해야 한다.
	alert := domain.Alert{ID: "w-err", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	queue := &mockAlertQueue{alerts: []domain.Alert{alert}}
	repo := &mockRepo{
		saveFn:   func(_ context.Context, _ domain.Alert) error { return nil },
		updateFn: func(_ context.Context, _ string, _ domain.AlertStatus, _ int, _ time.Time) error {
			return errors.New("db error")
		},
	}
	sender := &mockSender{}
	routeReader := &mockRouteReader{channels: []domain.Channel{{ID: "c1", Type: domain.ChannelTypeWebhook, Template: `{{ .Source }}`}}}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	// DB 에러에도 불구하고 send는 수행되어야 한다
	if sender.count.Load() == 0 {
		t.Error("expected send to be called despite UpdateDeliveryState error")
	}
}

func TestDeliveryWorker_DeliverSuccess(t *testing.T) {
	alert := domain.Alert{ID: "w1", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	queue := &mockAlertQueue{alerts: []domain.Alert{alert}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return nil }}
	sender := &mockSender{}
	routeReader := &mockRouteReader{channels: []domain.Channel{{ID: "c1", Type: domain.ChannelTypeWebhook, Template: `{{ .Source }}`}}}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	if sender.count.Load() == 0 {
		t.Error("expected at least one send call")
	}
}
