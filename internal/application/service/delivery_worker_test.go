package service_test

import (
	"context"
	"errors"
	"sync"
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

type mockRegistryFn struct{ senderFn func() output.AlertSender }

func (m *mockRegistryFn) Get(_ domain.ChannelType) (output.AlertSender, error) {
	return m.senderFn(), nil
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

type mockRouteReaderWithError struct{ err error }

func (m *mockRouteReaderWithError) GetChannels(_ context.Context, _ string) ([]domain.Channel, error) {
	return nil, m.err
}

func TestDeliveryWorker_NoRoute_Nacks(t *testing.T) {
	// source에 매핑된 route가 없을 때 alert을 nack 처리해야 한다
	alert := domain.Alert{ID: "no-route", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockAlertQueueWithNack{alert: alert, nackFn: func() error { nackCalled.Store(true); return nil }}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return nil }}
	registry := &mockRegistry{sender: &mockSender{}}
	routeReader := &mockRouteReaderWithError{err: errors.New("no route")}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if !nackCalled.Load() {
		t.Error("expected nack to be called when no route found")
	}
}

type mockAlertQueueWithNack struct {
	alert  domain.Alert
	nackFn func() error
	called atomic.Bool
}

func (m *mockAlertQueueWithNack) Enqueue(_ context.Context, _ domain.Alert) error { return nil }
func (m *mockAlertQueueWithNack) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	if m.called.Swap(true) {
		time.Sleep(10 * time.Millisecond)
		return domain.Alert{}, nil, nil, errors.New("empty")
	}
	return m.alert, func() error { return nil }, m.nackFn, nil
}

type mockSenderError struct{}

func (m *mockSenderError) Send(_ context.Context, _ domain.Channel, _ domain.Alert) error {
	return errors.New("send failed: render error")
}

func TestDeliveryWorker_SendError_MarksAsFailed(t *testing.T) {
	// Send가 에러를 반환하면 nack 처리되고 FAILED 상태로 기록되어야 한다
	alert := domain.Alert{ID: "send-err", Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockAlertQueueWithNack{alert: alert, nackFn: func() error { nackCalled.Store(true); return nil }}

	var mu sync.Mutex
	var updatedStatus domain.AlertStatus
	repo := &mockRepo{
		saveFn: func(_ context.Context, _ domain.Alert) error { return nil },
		updateFn: func(_ context.Context, _ string, s domain.AlertStatus, _ int, _ time.Time) error {
			mu.Lock()
			updatedStatus = s
			mu.Unlock()
			return nil
		},
	}
	routeReader := &mockRouteReader{channels: []domain.Channel{
		{ID: "c1", Type: domain.ChannelTypeWebhook, Template: `{}`, RetryCount: 1, RetryDelayMs: 10},
	}}
	registry := &mockRegistryFn{senderFn: func() output.AlertSender { return &mockSenderError{} }}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if !nackCalled.Load() {
		t.Error("expected nack when send fails")
	}
	mu.Lock()
	status := updatedStatus
	mu.Unlock()
	if status != domain.AlertStatusFailed {
		t.Errorf("status = %q, want FAILED", status)
	}
}

func TestDeliveryWorker_GracefulShutdown(t *testing.T) {
	// ctx 취소 후 Wait()이 반환되어야 한다 (타임아웃 없이)
	queue := &mockAlertQueue{}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return nil }}
	routeReader := &mockRouteReader{}
	registry := &mockRegistry{sender: &mockSender{}}

	ctx, cancel := context.WithCancel(context.Background())
	worker := service.NewDeliveryWorker(queue, repo, routeReader, registry)
	worker.Start(ctx, 2)

	cancel()

	done := make(chan struct{})
	go func() {
		worker.Wait()
		close(done)
	}()
	select {
	case <-done:
		// 정상 종료
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() did not return after context cancellation")
	}
}
