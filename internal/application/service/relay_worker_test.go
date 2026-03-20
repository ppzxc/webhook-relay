package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relaybox/internal/adapter/output/expression"
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

type mockRuleReader struct {
	rule    domain.Rule
	outputs []domain.Output
}

func (m *mockRuleReader) GetRule(_ context.Context, _ string) (domain.Rule, []domain.Output, error) {
	return m.rule, m.outputs, nil
}

type mockSender struct {
	count    atomic.Int32
	payloads [][]byte
	mu       sync.Mutex
}

func (m *mockSender) Send(_ context.Context, _ domain.Output, payload []byte) error {
	m.count.Add(1)
	m.mu.Lock()
	m.payloads = append(m.payloads, payload)
	m.mu.Unlock()
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

func newExprRegistry() output.ExpressionEngineRegistry {
	reg := expression.NewInMemoryExpressionEngineRegistry()
	celEng, err := expression.NewCELEngine()
	if err != nil {
		panic("NewCELEngine: " + err.Error())
	}
	reg.Register(celEng)
	reg.Register(expression.NewExprEngine())
	return reg
}

func TestRelayWorker_UpdateDeliveryState_ErrorDoesNotBreakWorker(t *testing.T) {
	msg := domain.Message{ID: "w-err", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{
		saveFn:   func(_ context.Context, _ domain.Message) error { return nil },
		updateFn: func(_ context.Context, _ string, _ domain.MessageStatus, _ int, _ time.Time) error {
			return errors.New("db error")
		},
	}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	if sender.count.Load() == 0 {
		t.Error("expected send to be called despite UpdateDeliveryState error")
	}
}

func TestRelayWorker_DeliverSuccess(t *testing.T) {
	msg := domain.Message{ID: "w1", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{"host":"server1"}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)

	time.Sleep(150 * time.Millisecond)
	if sender.count.Load() == 0 {
		t.Error("expected at least one send call")
	}
}

type mockRuleReaderWithError struct{ err error }

func (m *mockRuleReaderWithError) GetRule(_ context.Context, _ string) (domain.Rule, []domain.Output, error) {
	return domain.Rule{}, nil, m.err
}

func TestRelayWorker_NoRule_Nacks(t *testing.T) {
	msg := domain.Message{ID: "no-rule", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockMessageQueueWithNack{msg: msg, nackFn: func() error { nackCalled.Store(true); return nil }}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	registry := &mockRegistry{sender: &mockSender{}}
	ruleReader := &mockRuleReaderWithError{err: errors.New("no rule")}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
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

func (m *mockSenderError) Send(_ context.Context, _ domain.Output, _ []byte) error {
	return errors.New("send failed: render error")
}

func TestRelayWorker_SendError_MarksAsFailed(t *testing.T) {
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
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, RetryCount: 1, RetryDelayMs: 10}},
	}
	registry := &mockRegistryFn{senderFn: func() output.OutputSender { return &mockSenderError{} }}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
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
	queue := &mockMessageQueue{}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	ruleReader := &mockRuleReader{}
	registry := &mockRegistry{sender: &mockSender{}}

	ctx, cancel := context.WithCancel(context.Background())
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 2)

	cancel()

	done := make(chan struct{})
	go func() {
		worker.Wait()
		close(done)
	}()
	select {
	case <-done:
		// normal shutdown
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() did not return after context cancellation")
	}
}

// --- New expression engine tests ---

func TestRelayWorker_FilterTrue_Passes(t *testing.T) {
	msg := domain.Message{ID: "f-true", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel", Filter: `data.input == "BESZEL"`},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Error("expected send when filter passes")
	}
}

func TestRelayWorker_FilterFalse_Skips(t *testing.T) {
	msg := domain.Message{ID: "f-false", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var ackCalled atomic.Bool
	queue := &mockMessageQueueWithAckNack{
		msg:    msg,
		ackFn:  func() error { ackCalled.Store(true); return nil },
		nackFn: func() error { return nil },
	}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel", Filter: `data.input == "NONEXISTENT"`},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() != 0 {
		t.Error("expected no send when filter rejects")
	}
	if !ackCalled.Load() {
		t.Error("expected ack when filter rejects (message processed)")
	}
}

type mockMessageQueueWithAckNack struct {
	msg    domain.Message
	ackFn  func() error
	nackFn func() error
	called atomic.Bool
}

func (m *mockMessageQueueWithAckNack) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockMessageQueueWithAckNack) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	if m.called.Swap(true) {
		time.Sleep(10 * time.Millisecond)
		return domain.Message{}, nil, nil, errors.New("empty")
	}
	return m.msg, m.ackFn, m.nackFn, nil
}

func TestRelayWorker_EmptyFilter_PassesAll(t *testing.T) {
	msg := domain.Message{ID: "no-filter", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel", Filter: ""},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Error("expected send when no filter set")
	}
}

func TestRelayWorker_EmptyRouting_AllOutputs(t *testing.T) {
	msg := domain.Message{ID: "all-out", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule: domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{
			{ID: "c1", Type: domain.OutputTypeWebhook},
			{ID: "c2", Type: domain.OutputTypeWebhook},
		},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() != 2 {
		t.Errorf("expected 2 sends (all outputs), got %d", sender.count.Load())
	}
}

func TestRelayWorker_Routing_MatchesCorrectOutputs(t *testing.T) {
	msg := domain.Message{ID: "route-msg", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule: domain.Rule{
			InputID: "beszel",
			Routing: []domain.RouteCondition{
				{Condition: `data.input == "BESZEL"`, OutputIDs: []string{"c1"}},
				{Condition: `data.input == "DOZZLE"`, OutputIDs: []string{"c2"}},
			},
		},
		outputs: []domain.Output{
			{ID: "c1", Type: domain.OutputTypeWebhook},
			{ID: "c2", Type: domain.OutputTypeWebhook},
		},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() != 1 {
		t.Errorf("expected 1 send (only c1 matched), got %d", sender.count.Load())
	}
}

func TestRelayWorker_TemplateExpressions_BuildPayload(t *testing.T) {
	msg := domain.Message{
		ID: "tmpl-msg", Input: domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule: domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{
			ID: "c1", Type: domain.OutputTypeWebhook,
			Template: map[string]string{
				"src": `data.input`,
				"msg": `"alert from " + data.input`,
			},
		}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Fatal("expected send call")
	}

	sender.mu.Lock()
	payload := sender.payloads[0]
	sender.mu.Unlock()

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if result["src"] != "BESZEL" {
		t.Errorf("src = %v, want BESZEL", result["src"])
	}
	if result["msg"] != "alert from BESZEL" {
		t.Errorf("msg = %v, want 'alert from BESZEL'", result["msg"])
	}
}

func TestRelayWorker_NoTemplate_UsesRawPayload(t *testing.T) {
	msg := domain.Message{
		ID: "raw-msg", Input: domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Fatal("expected send call")
	}

	sender.mu.Lock()
	payload := sender.payloads[0]
	sender.mu.Unlock()

	if string(payload) != `{"host":"server1"}` {
		t.Errorf("payload = %q, want raw payload", payload)
	}
}

func TestRelayWorker_Mapping_EnrichesData(t *testing.T) {
	msg := domain.Message{
		ID: "map-msg", Input: domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule: domain.Rule{
			InputID: "beszel",
			Mapping: map[string]string{
				"upperInput": `"[" + data.input + "]"`,
			},
		},
		outputs: []domain.Output{{
			ID: "c1", Type: domain.OutputTypeWebhook,
			Template: map[string]string{
				"tag": `data.upperInput`,
			},
		}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Fatal("expected send call")
	}

	sender.mu.Lock()
	payload := sender.payloads[0]
	sender.mu.Unlock()

	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["tag"] != "[BESZEL]" {
		t.Errorf("tag = %v, want [BESZEL]", result["tag"])
	}
}
