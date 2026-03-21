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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
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

// mockCountingSenderError counts Send calls and always returns an error.
type mockCountingSenderError struct {
	count atomic.Int32
}

func (m *mockCountingSenderError) Send(_ context.Context, _ domain.Output, _ []byte) error {
	m.count.Add(1)
	return errors.New("send error")
}

type mockRegistryCountingError struct{ sender *mockCountingSenderError }

func (m *mockRegistryCountingError) Get(_ domain.OutputType) (output.OutputSender, error) {
	return m.sender, nil
}

func TestRelayWorker_CustomRetryDefaults(t *testing.T) {
	msg := domain.Message{ID: "retry-custom", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockCountingSenderError{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, RetryCount: 0, RetryDelayMs: 0}},
	}
	registry := &mockRegistryCountingError{sender: sender}

	cfg := service.RelayWorkerConfig{DefaultRetryCount: 1, DefaultRetryDelayMs: 10}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if got := sender.count.Load(); got != 1 {
		t.Errorf("sender called %d times, want exactly 1 (DefaultRetryCount=1)", got)
	}
}

func TestRelayWorker_ZeroConfig_UsesDefaults(t *testing.T) {
	msg := domain.Message{ID: "zero-cfg", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: sender}

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.RelayWorkerConfig{})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Error("expected at least one send call with zero config (uses defaults)")
	}
}

func TestRelayWorker_PerOutputRetry_OverridesDefault(t *testing.T) {
	msg := domain.Message{ID: "per-out", Input: domain.InputTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockCountingSenderError{}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, RetryCount: 1, RetryDelayMs: 10}},
	}
	registry := &mockRegistryCountingError{sender: sender}

	// DefaultRetryCount=5 but per-output RetryCount=1 should win.
	cfg := service.RelayWorkerConfig{DefaultRetryCount: 5, DefaultRetryDelayMs: 10}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if got := sender.count.Load(); got != 1 {
		t.Errorf("sender called %d times, want 1 (per-output RetryCount=1 overrides default=5)", got)
	}
}

func TestRelayWorker_InvalidTransition_SkipsUpdate(t *testing.T) {
	// DELIVERED→DELIVERED is an invalid transition; UpdateDeliveryState must NOT be called.
	msg := domain.Message{
		ID: "already-delivered", Input: domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusDelivered, // already terminal
		Version: 1,
	}
	var ackCalled atomic.Bool
	queue := &mockMessageQueueWithAckNack{
		msg:    msg,
		ackFn:  func() error { ackCalled.Store(true); return nil },
		nackFn: func() error { return nil },
	}

	var updateCount atomic.Int32
	repo := &mockRepo{
		saveFn: func(_ context.Context, _ domain.Message) error { return nil },
		updateFn: func(_ context.Context, _ string, _ domain.MessageStatus, _ int, _ time.Time) error {
			updateCount.Add(1)
			return nil
		},
	}
	ruleReader := &mockRuleReader{
		rule:    domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook}},
	}
	registry := &mockRegistry{sender: &mockSender{}}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if n := updateCount.Load(); n != 0 {
		t.Errorf("UpdateDeliveryState called %d times, want 0 (invalid transition must be skipped)", n)
	}
	if !ackCalled.Load() {
		t.Error("expected ack to be called regardless of invalid transition")
	}
}

func TestRelayWorker_ParsedDataDoesNotOverrideBuiltinKeys(t *testing.T) {
	// ParsedData contains "input": "HACKED" — if this overwrites the builtin
	// "input" key, the filter `data.input == "BESZEL"` will fail and the sender
	// will never be called. The fix must protect builtin keys from ParsedData.
	msg := domain.Message{
		ID:      "key-collision",
		Input:   domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusPending,
		Version: 1,
		ParsedData: map[string]any{
			"input": "HACKED", // must NOT overwrite builtin "input" = "BESZEL"
		},
	}
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

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Error("builtin key 'input' was overwritten by ParsedData — filter failed when it should have passed")
	}
}

func TestRelayWorker_NestedTemplateKeys(t *testing.T) {
	// template with dot-notation keys must produce nested JSON.
	// e.g. "content.type" + "content.text" → {"content": {"type": "text", "text": "..."}}
	msg := domain.Message{
		ID:      "nested-tmpl",
		Input:   domain.InputTypeBeszel,
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending,
		Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		rule: domain.Rule{InputID: "beszel"},
		outputs: []domain.Output{{
			ID:   "naver-works",
			Type: domain.OutputTypeWebhook,
			Template: map[string]string{
				"content.type": `"text"`,
				"content.text": `data.input + " alert: " + data.payload`,
			},
		}},
	}
	registry := &mockRegistry{sender: sender}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.DefaultRelayWorkerConfig())
	worker.Start(ctx, 1)
	time.Sleep(150 * time.Millisecond)

	if sender.count.Load() == 0 {
		t.Fatal("expected sender to be called")
	}

	sender.mu.Lock()
	payload := sender.payloads[0]
	sender.mu.Unlock()

	// Verify nested structure: {"content": {"type": "text", "text": "BESZEL alert: ..."}}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	content, ok := got["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected got[\"content\"] to be map[string]any, got %T; payload=%s", got["content"], payload)
	}
	if content["type"] != "text" {
		t.Errorf("content.type = %q, want %q", content["type"], "text")
	}
	wantPrefix := "BESZEL alert: "
	text, _ := content["text"].(string)
	if len(text) == 0 || text[:len(wantPrefix)] != wantPrefix {
		t.Errorf("content.text = %q, want prefix %q", text, wantPrefix)
	}
}
