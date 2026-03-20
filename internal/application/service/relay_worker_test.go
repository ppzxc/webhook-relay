package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	"relaybox/internal/domain"
)

type mockMessageQueue struct {
	messages []domain.Message
	idx      int
}

func (m *mockMessageQueue) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockMessageQueue) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	if m.idx >= len(m.messages) {
		time.Sleep(10 * time.Millisecond)
		return domain.Message{}, nil, nil, errors.New("empty")
	}
	a := m.messages[m.idx]
	m.idx++
	return a, func() error { return nil }, func() error { return nil }, nil
}

type mockRuleReader struct{ outputs []domain.Output }

func (m *mockRuleReader) GetOutputs(_ context.Context, _ string) ([]domain.Output, error) {
	return m.outputs, nil
}

type mockSender struct{ count atomic.Int32 }

func (m *mockSender) Send(_ context.Context, _ domain.Output, _ domain.Message) error {
	m.count.Add(1)
	return nil
}

type mockRegistry struct{ sender *mockSender }

func (m *mockRegistry) Get(_ domain.OutputType) (output.OutputSender, error) {
	return m.sender, nil
}

type mockRegistryFn struct{ senderFn func() output.OutputSender }

func (m *mockRegistryFn) Get(_ domain.OutputType) (output.OutputSender, error) {
	return m.senderFn(), nil
}

func TestRelayWorker_UpdateDeliveryState_ErrorDoesNotBreakWorker(t *testing.T) {
	// UpdateDeliveryState가 에러를 반환해도 워커가 정상 동작(send 완료, 패닉 없음)해야 한다.
	msg := domain.Message{ID: "w-err", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{
		saveFn:   func(_ context.Context, _ domain.Message) error { return nil },
		updateFn: func(_ context.Context, _ string, _ domain.MessageStatus, _ int, _ time.Time) error {
			return errors.New("db error")
		},
	}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Template: `{{ .Source }}`}}}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	// DB 에러에도 불구하고 send는 수행되어야 한다
	if sender.count.Load() == 0 {
		t.Error("expected send to be called despite UpdateDeliveryState error")
	}
}

func TestRelayWorker_DeliverSuccess(t *testing.T) {
	msg := domain.Message{ID: "w1", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Template: `{{ .Source }}`}}}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	if sender.count.Load() == 0 {
		t.Error("expected at least one send call")
	}
}

type mockRuleReaderWithError struct{ err error }

func (m *mockRuleReaderWithError) GetOutputs(_ context.Context, _ string) ([]domain.Output, error) {
	return nil, m.err
}

func TestRelayWorker_NoRule_Nacks(t *testing.T) {
	// input에 매핑된 rule이 없을 때 message를 nack 처리해야 한다
	msg := domain.Message{ID: "no-rule", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockMessageQueueWithNack{msg: msg, nackFn: func() error { nackCalled.Store(true); return nil }}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	registry := &mockRegistry{sender: &mockSender{}}
	ruleReader := &mockRuleReaderWithError{err: errors.New("no rule")}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if !nackCalled.Load() {
		t.Error("expected nack to be called when no rule found")
	}
}

type mockMessageQueueWithNack struct {
	msg    domain.Message
	nackFn func() error
	called atomic.Bool
}

func (m *mockMessageQueueWithNack) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockMessageQueueWithNack) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	if m.called.Swap(true) {
		time.Sleep(10 * time.Millisecond)
		return domain.Message{}, nil, nil, errors.New("empty")
	}
	return m.msg, func() error { return nil }, m.nackFn, nil
}

type mockSenderError struct{}

func (m *mockSenderError) Send(_ context.Context, _ domain.Output, _ domain.Message) error {
	return errors.New("send failed: render error")
}

func TestRelayWorker_SendError_MarksAsFailed(t *testing.T) {
	// Send가 에러를 반환하면 nack 처리되고 FAILED 상태로 기록되어야 한다
	msg := domain.Message{ID: "send-err", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockMessageQueueWithNack{msg: msg, nackFn: func() error { nackCalled.Store(true); return nil }}

	var mu sync.Mutex
	var updatedStatus domain.MessageStatus
	repo := &mockRepo{
		saveFn: func(_ context.Context, _ domain.Message) error { return nil },
		updateFn: func(_ context.Context, _ string, s domain.MessageStatus, _ int, _ time.Time) error {
			mu.Lock()
			updatedStatus = s
			mu.Unlock()
			return nil
		},
	}
	ruleReader := &mockRuleReader{outputs: []domain.Output{
		{ID: "c1", Type: domain.OutputTypeWebhook, Template: `{}`, RetryCount: 1, RetryDelayMs: 10},
	}}
	registry := &mockRegistryFn{senderFn: func() output.OutputSender { return &mockSenderError{} }}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if !nackCalled.Load() {
		t.Error("expected nack when send fails")
	}
	mu.Lock()
	status := updatedStatus
	mu.Unlock()
	if status != domain.MessageStatusFailed {
		t.Errorf("status = %q, want FAILED", status)
	}
}

func TestRelayWorker_GracefulShutdown(t *testing.T) {
	// ctx 취소 후 Wait()이 반환되어야 한다 (타임아웃 없이)
	queue := &mockMessageQueue{}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	ruleReader := &mockRuleReader{}
	registry := &mockRegistry{sender: &mockSender{}}

	ctx, cancel := context.WithCancel(context.Background())
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry)
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
