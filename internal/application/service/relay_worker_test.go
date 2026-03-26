package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"relaybox/internal/adapter/output/expression"
	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	"relaybox/internal/domain"
	"relaybox/internal/testutil"
)

type mockMessageQueue struct {
	messages []domain.Message
	idx      int
}

func (m *mockMessageQueue) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockMessageQueue) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	if m.idx >= len(m.messages) {
		time.Sleep(10 * time.Millisecond)
		return domain.Message{}, nil, nil, output.ErrQueueEmpty
	}
	a := m.messages[m.idx]
	m.idx++
	return a, func() error { return nil }, func() error { return nil }, nil
}

type mockRuleReader struct {
	engine  string
	entries []domain.RuleEntry
}

func (m *mockRuleReader) GetRules(_ context.Context, _ string) (string, []domain.RuleEntry, error) {
	return m.engine, m.entries, nil
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

// processedChan returns a buffered channel and an OnProcessed callback that signals it.
func processedChan() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	return ch, func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// waitForProcessed blocks until the processed channel receives a signal or the test times out.
func waitForProcessed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for message processing")
	}
}

func TestRelayWorker_UpdateDeliveryState_ErrorDoesNotBreakWorker(t *testing.T) {
	msg := domain.Message{ID: "w-err", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{
		saveFn: func(_ context.Context, _ domain.Message) error { return nil },
		updateFn: func(_ context.Context, _ string, _ domain.MessageStatus, _ int, _ time.Time) error {
			return errors.New("db error")
		},
	}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("expected send to be called despite UpdateDeliveryState error")
	}
}

func TestRelayWorker_DeliverSuccess(t *testing.T) {
	msg := domain.Message{ID: "w1", Input: "beszel", Payload: domain.RawPayload(`{"host":"server1"}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("expected at least one send call")
	}
}

type mockRuleReaderWithError struct{ err error }

func (m *mockRuleReaderWithError) GetRules(_ context.Context, _ string) (string, []domain.RuleEntry, error) {
	return "", nil, m.err
}

func TestRelayWorker_NoRule_Nacks(t *testing.T) {
	msg := domain.Message{ID: "no-rule", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var nackCalled atomic.Bool
	queue := &mockMessageQueueWithNack{msg: msg, nackFn: func() error { nackCalled.Store(true); return nil }}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	registry := &mockRegistry{sender: &mockSender{}}
	ruleReader := &mockRuleReaderWithError{err: errors.New("no rule")}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
		return domain.Message{}, nil, nil, output.ErrQueueEmpty
	}
	return m.msg, func() error { return nil }, m.nackFn, nil
}

type mockSenderError struct{}

func (m *mockSenderError) Send(_ context.Context, _ domain.Output, _ []byte) error {
	return errors.New("send failed: render error")
}

func TestRelayWorker_SendError_MarksAsFailed(t *testing.T) {
	msg := domain.Message{ID: "send-err", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
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
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL", RetryCount: 1, RetryDelayMs: 10}},
		}},
	}
	registry := &mockRegistryFn{senderFn: func() output.OutputSender { return &mockSenderError{} }}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
	msg := domain.Message{ID: "f-true", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{Filter: `data.input == "beszel"`},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("expected send when filter passes")
	}
}

func TestRelayWorker_FilterFalse_Skips(t *testing.T) {
	msg := domain.Message{ID: "f-false", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	var ackCalled atomic.Bool
	queue := &mockMessageQueueWithAckNack{
		msg:    msg,
		ackFn:  func() error { ackCalled.Store(true); return nil },
		nackFn: func() error { return nil },
	}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{Filter: `data.input == "NONEXISTENT"`},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
	msg := domain.Message{ID: "no-filter", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{Filter: ""},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("expected send when no filter set")
	}
}

func TestRelayWorker_EmptyRouting_AllOutputs(t *testing.T) {
	msg := domain.Message{ID: "all-out", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule: domain.Rule{},
			Outputs: []domain.Output{
				{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"},
				{ID: "c2", Type: domain.OutputTypeWebhook, Engine: "CEL"},
			},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() != 2 {
		t.Errorf("expected 2 sends (all outputs), got %d", sender.count.Load())
	}
}

func TestRelayWorker_Routing_MatchesCorrectOutputs(t *testing.T) {
	msg := domain.Message{ID: "route-msg", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule: domain.Rule{
				Routing: []domain.RouteCondition{
					{Condition: `data.input == "beszel"`, OutputIDs: []string{"c1"}},
					{Condition: `data.input == "dozzle"`, OutputIDs: []string{"c2"}},
				},
			},
			Outputs: []domain.Output{
				{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"},
				{ID: "c2", Type: domain.OutputTypeWebhook, Engine: "CEL"},
			},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() != 1 {
		t.Errorf("expected 1 send (only c1 matched), got %d", sender.count.Load())
	}
}

func TestRelayWorker_TemplateExpressions_BuildPayload(t *testing.T) {
	msg := domain.Message{
		ID: "tmpl-msg", Input: "beszel",
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule: domain.Rule{},
			Outputs: []domain.Output{{
				ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL",
				Template: map[string]string{
					"src": `data.input`,
					"msg": `"alert from " + data.input`,
				},
			}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
	if result["src"] != "beszel" {
		t.Errorf("src = %v, want beszel", result["src"])
	}
	if result["msg"] != "alert from beszel" {
		t.Errorf("msg = %v, want 'alert from beszel'", result["msg"])
	}
}

func TestRelayWorker_NoTemplate_UsesRawPayload(t *testing.T) {
	msg := domain.Message{
		ID: "raw-msg", Input: "beszel",
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
		ID: "map-msg", Input: "beszel",
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule: domain.Rule{
				Mapping: map[string]string{
					"upperInput": `"[" + data.input + "]"`,
				},
			},
			Outputs: []domain.Output{{
				ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL",
				Template: map[string]string{
					"tag": `data.upperInput`,
				},
			}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

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
	if result["tag"] != "[beszel]" {
		t.Errorf("tag = %v, want [beszel]", result["tag"])
	}
}

func TestRelayWorker_LogsInfoOnProcessing(t *testing.T) {
	h := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(orig)

	msgObj := domain.Message{
		ID: "log-test", Input: "beszel",
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending, Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msgObj}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	hasProcessingLog := false
	hasDeliveredLog := false
	for _, rec := range h.Records() {
		if rec.Level == slog.LevelInfo && rec.Msg == "processing message" {
			hasProcessingLog = true
		}
		if rec.Level == slog.LevelInfo && rec.Msg == "message delivered" {
			hasDeliveredLog = true
		}
	}
	if !hasProcessingLog {
		t.Error(`expected INFO "processing message" log record not found`)
	}
	if !hasDeliveredLog {
		t.Error(`expected INFO "message delivered" log record not found`)
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
	msg := domain.Message{ID: "retry-custom", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockCountingSenderError{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL", RetryCount: 0, RetryDelayMs: 0}},
		}},
	}
	registry := &mockRegistryCountingError{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.RelayWorkerConfig{DefaultRetryCount: 1, DefaultRetryDelay: 10 * time.Millisecond, Hooks: service.RelayWorkerHooks{OnProcessed: onProcessed}}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if got := sender.count.Load(); got != 1 {
		t.Errorf("sender called %d times, want exactly 1 (DefaultRetryCount=1)", got)
	}
}

func TestRelayWorker_ZeroConfig_UsesDefaults(t *testing.T) {
	msg := domain.Message{ID: "zero-cfg", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), service.RelayWorkerConfig{Hooks: service.RelayWorkerHooks{OnProcessed: onProcessed}})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("expected at least one send call with zero config (uses defaults)")
	}
}

func TestRelayWorker_PerOutputRetry_OverridesDefault(t *testing.T) {
	msg := domain.Message{ID: "per-out", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockCountingSenderError{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL", RetryCount: 1, RetryDelayMs: 10}},
		}},
	}
	registry := &mockRegistryCountingError{sender: sender}

	// DefaultRetryCount=5 but per-output RetryCount=1 should win.
	processed, onProcessed := processedChan()
	cfg := service.RelayWorkerConfig{DefaultRetryCount: 5, DefaultRetryDelay: 10 * time.Millisecond, Hooks: service.RelayWorkerHooks{OnProcessed: onProcessed}}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if got := sender.count.Load(); got != 1 {
		t.Errorf("sender called %d times, want 1 (per-output RetryCount=1 overrides default=5)", got)
	}
}

func TestRelayWorker_InvalidTransition_SkipsUpdate(t *testing.T) {
	// DELIVERED→DELIVERED is an invalid transition; UpdateDeliveryState must NOT be called.
	msg := domain.Message{
		ID: "already-delivered", Input: "beszel",
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
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: &mockSender{}}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if n := updateCount.Load(); n != 0 {
		t.Errorf("UpdateDeliveryState called %d times, want 0 (invalid transition must be skipped)", n)
	}
	if !ackCalled.Load() {
		t.Error("expected ack to be called regardless of invalid transition")
	}
}

func TestRelayWorker_ParsedDataDoesNotOverrideBuiltinKeys(t *testing.T) {
	msg := domain.Message{
		ID:      "key-collision",
		Input:   "beszel",
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusPending,
		Version: 1,
		ParsedData: map[string]any{
			"input": "HACKED", // must NOT overwrite builtin "input" = "beszel"
		},
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{Filter: `data.input == "beszel"`},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Error("builtin key 'input' was overwritten by ParsedData — filter failed when it should have passed")
	}
}

func TestRelayWorker_NestedTemplateKeys(t *testing.T) {
	msg := domain.Message{
		ID:      "nested-tmpl",
		Input:   "beszel",
		Payload: domain.RawPayload(`{"host":"server1"}`),
		Status:  domain.MessageStatusPending,
		Version: 1,
	}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule: domain.Rule{},
			Outputs: []domain.Output{{
				ID:     "naver-works",
				Type:   domain.OutputTypeWebhook,
				Engine: "CEL",
				Template: map[string]string{
					"content.type": `"text"`,
					"content.text": `data.input + " alert: " + data.payload`,
				},
			}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	if sender.count.Load() == 0 {
		t.Fatal("expected sender to be called")
	}

	sender.mu.Lock()
	payload := sender.payloads[0]
	sender.mu.Unlock()

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
	wantPrefix := "beszel alert: "
	text, _ := content["text"].(string)
	if len(text) == 0 || text[:len(wantPrefix)] != wantPrefix {
		t.Errorf("content.text = %q, want prefix %q", text, wantPrefix)
	}
}

func TestRelayWorker_MultipleRuleEntries_EachProcessedIndependently(t *testing.T) {
	msg := domain.Message{ID: "multi-rule", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockMessageQueue{messages: []domain.Message{msg}}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{
			{
				Rule:    domain.Rule{},
				Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
			},
			{
				Rule:    domain.Rule{Filter: `data.input == "NONEXISTENT"`},
				Outputs: []domain.Output{{ID: "c2", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
			},
			{
				Rule:    domain.Rule{},
				Outputs: []domain.Output{{ID: "c3", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
			},
		},
	}
	registry := &mockRegistry{sender: sender}

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	// entry 1 passes (no filter) → c1 delivered
	// entry 2 filtered out (NONEXISTENT) → skipped
	// entry 3 passes (no filter) → c3 delivered
	if got := sender.count.Load(); got != 2 {
		t.Errorf("expected 2 sends (entry1 + entry3, entry2 filtered), got %d", got)
	}
}

// mockErrorQueue returns a non-ErrQueueEmpty error every time Dequeue is called.
type mockErrorQueue struct{ err error }

func (m *mockErrorQueue) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockErrorQueue) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	time.Sleep(10 * time.Millisecond)
	return domain.Message{}, nil, nil, m.err
}

func TestRelayWorker_DequeueError_LogsWarn(t *testing.T) {
	captureH := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(captureH))
	defer slog.SetDefault(orig)

	dequeueErr := errors.New("disk read failed")
	queue := &mockErrorQueue{err: dequeueErr}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	ruleReader := &mockRuleReader{}
	registry := &mockRegistry{sender: &mockSender{}}

	errSeen := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())

	cfg := service.DefaultRelayWorkerConfig()
	cfg.Hooks.OnLoopError = func(err error) {
		if errors.Is(err, dequeueErr) {
			select {
			case errSeen <- struct{}{}:
			default:
			}
		}
	}
	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)

	select {
	case <-errSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for loop error callback")
	}
	cancel()
	worker.Wait()

	records := captureH.Records()
	found := false
	for _, r := range records {
		if r.Level == slog.LevelWarn && r.Msg == "processOne failed" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected WARN log 'processOne failed' when Dequeue returns non-ErrQueueEmpty error, got records: %v", records)
	}
}

// mockPanicThenNormalQueue: 첫 Dequeue에서 panic, 이후 정상 메시지 반환.
type mockPanicThenNormalQueue struct {
	msg     domain.Message
	paniced atomic.Bool
	done    atomic.Bool
}

func (m *mockPanicThenNormalQueue) Enqueue(_ context.Context, _ domain.Message) error { return nil }
func (m *mockPanicThenNormalQueue) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	if !m.paniced.Swap(true) {
		panic("simulated worker panic")
	}
	if m.done.Swap(true) {
		time.Sleep(10 * time.Millisecond)
		return domain.Message{}, nil, nil, output.ErrQueueEmpty
	}
	return m.msg, func() error { return nil }, func() error { return nil }, nil
}

func TestRelayWorker_PanicRecovery(t *testing.T) {
	msg := domain.Message{ID: "panic-msg", Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1}
	queue := &mockPanicThenNormalQueue{msg: msg}
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return nil }}
	sender := &mockSender{}
	ruleReader := &mockRuleReader{
		engine: "CEL",
		entries: []domain.RuleEntry{{
			Rule:    domain.Rule{},
			Outputs: []domain.Output{{ID: "c1", Type: domain.OutputTypeWebhook, Engine: "CEL"}},
		}},
	}
	registry := &mockRegistry{sender: sender}

	captureH := &testutil.CaptureHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(captureH))
	defer slog.SetDefault(orig)

	processed, onProcessed := processedChan()
	cfg := service.DefaultRelayWorkerConfig()
	cfg.PollBackoff = 10 * time.Millisecond
	cfg.Hooks.OnProcessed = onProcessed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := service.NewRelayWorker(queue, repo, ruleReader, registry, newExprRegistry(), cfg)
	worker.Start(ctx, 1)
	waitForProcessed(t, processed)

	// panic 후에도 메시지가 처리되어야 한다
	if sender.count.Load() == 0 {
		t.Error("expected worker to recover from panic and process message")
	}

	// ERROR 레벨 "relay worker panic recovered" 로그가 있어야 한다
	records := captureH.Records()
	found := false
	for _, r := range records {
		if r.Level == slog.LevelError && r.Msg == "relay worker panic recovered" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ERROR log 'relay worker panic recovered', got records: %v", records)
	}
}
