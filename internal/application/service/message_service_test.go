package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"relaybox/internal/application/port/input"
	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	"relaybox/internal/domain"
)

type mockRepo struct {
	saveFn      func(context.Context, domain.Message) error
	updateFn    func(context.Context, string, domain.MessageStatus, int, time.Time) error
	findByIDFn  func(string) (domain.Message, error)
}

func (m *mockRepo) Save(ctx context.Context, a domain.Message) error { return m.saveFn(ctx, a) }
func (m *mockRepo) UpdateDeliveryState(ctx context.Context, id string, s domain.MessageStatus, retry int, t time.Time) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, s, retry, t)
	}
	return nil
}
func (m *mockRepo) FindByID(_ context.Context, id string) (domain.Message, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(id)
	}
	return domain.Message{}, nil
}
func (m *mockRepo) FindByInput(_ context.Context, _ string, _, _ int) ([]domain.Message, error) {
	return nil, nil
}

type mockQueue struct {
	enqueueFn func(context.Context, domain.Message) error
}

func (m *mockQueue) Enqueue(ctx context.Context, a domain.Message) error { return m.enqueueFn(ctx, a) }
func (m *mockQueue) Dequeue(_ context.Context) (domain.Message, output.AckFunc, output.NackFunc, error) {
	return domain.Message{}, nil, nil, nil
}

type mockParser struct {
	result map[string]any
	err    error
	typ    string
}

func (m *mockParser) Parse(_ string, _ []byte) (map[string]any, error) { return m.result, m.err }
func (m *mockParser) Type() string                                      { return m.typ }

type mockParserRegistry struct {
	parsers map[string]input.Parser
}

func (m *mockParserRegistry) Get(t string) (input.Parser, error) {
	p, ok := m.parsers[t]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func TestMessageService_Receive_Success(t *testing.T) {
	var saved domain.Message
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Message) error { saved = a; return nil }}
	var enqueued domain.Message
	queue := &mockQueue{enqueueFn: func(_ context.Context, a domain.Message) error { enqueued = a; return nil }}

	svc := service.NewMessageService(repo, queue, nil, nil)
	id, err := svc.Receive(context.Background(), "beszel", "application/json", []byte(`{"host":"srv1"}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if id == "" {
		t.Error("returned ID should not be empty")
	}
	if saved.Input != "beszel" {
		t.Errorf("input = %q, want BESZEL", saved.Input)
	}
	if saved.Status != domain.MessageStatusPending {
		t.Errorf("status = %q, want PENDING", saved.Status)
	}
	if saved.ID != id {
		t.Errorf("saved.ID = %q, want returned ID %q", saved.ID, id)
	}
	if enqueued.ID != saved.ID {
		t.Errorf("enqueued ID != saved ID")
	}
}

func TestMessageService_Receive_SaveError(t *testing.T) {
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Message) error { return errors.New("db err") }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}

	svc := service.NewMessageService(repo, queue, nil, nil)
	if _, err := svc.Receive(context.Background(), "beszel", "application/json", []byte(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}

func TestMessageService_Receive_WithParser(t *testing.T) {
	var saved domain.Message
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Message) error { saved = a; return nil }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}

	registry := &mockParserRegistry{
		parsers: map[string]input.Parser{
			"JSON": &mockParser{
				result: map[string]any{"host": "srv1", "port": float64(8080)},
				typ:    "JSON",
			},
		},
	}

	parserTypes := map[string]string{
		"beszel": "JSON",
	}

	svc := service.NewMessageService(repo, queue, parserTypes, registry)
	_, err := svc.Receive(context.Background(), "beszel", "application/json", []byte(`{"host":"srv1","port":8080}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}

	if saved.ParsedData == nil {
		t.Fatal("ParsedData should not be nil when parser is configured")
	}
	if saved.ParsedData["host"] != "srv1" {
		t.Errorf("ParsedData[host] = %v, want srv1", saved.ParsedData["host"])
	}
}

func TestMessageService_Receive_WithoutParser(t *testing.T) {
	var saved domain.Message
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Message) error { saved = a; return nil }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}

	svc := service.NewMessageService(repo, queue, nil, nil)
	_, err := svc.Receive(context.Background(), "beszel", "application/json", []byte(`{"host":"srv1"}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}

	if saved.ParsedData != nil {
		t.Errorf("ParsedData should be nil when no parser is configured, got %v", saved.ParsedData)
	}
}

func TestMessageService_GetByID_Success(t *testing.T) {
	want := domain.Message{
		ID:     "msg-1",
		Input:  "beszel",
		Status: domain.MessageStatusPending,
	}
	var receivedID string
	repo := &mockRepo{
		saveFn: func(_ context.Context, _ domain.Message) error { return nil },
		findByIDFn: func(id string) (domain.Message, error) {
			receivedID = id
			return want, nil
		},
	}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}
	svc := service.NewMessageService(repo, queue, nil, nil)

	got, err := svc.GetByID(context.Background(), "msg-1")
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if receivedID != "msg-1" {
		t.Errorf("repo received id = %q, want %q", receivedID, "msg-1")
	}
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
}

func TestMessageService_GetByID_NotFound(t *testing.T) {
	repo := &mockRepo{
		saveFn:     func(_ context.Context, _ domain.Message) error { return nil },
		findByIDFn: func(_ string) (domain.Message, error) { return domain.Message{}, domain.ErrMessageNotFound },
	}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}
	svc := service.NewMessageService(repo, queue, nil, nil)

	_, err := svc.GetByID(context.Background(), "nonexistent")
	if !errors.Is(err, domain.ErrMessageNotFound) {
		t.Fatalf("expected ErrMessageNotFound, got %v", err)
	}
}

func TestMessageService_Receive_ParserFailsGracefully(t *testing.T) {
	var saved domain.Message
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Message) error { saved = a; return nil }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Message) error { return nil }}

	registry := &mockParserRegistry{
		parsers: map[string]input.Parser{
			"JSON": &mockParser{
				err: errors.New("parse error"),
				typ: "JSON",
			},
		},
	}

	parserTypes := map[string]string{
		"beszel": "JSON",
	}

	svc := service.NewMessageService(repo, queue, parserTypes, registry)
	_, err := svc.Receive(context.Background(), "beszel", "application/json", []byte(`not-json`))
	if err != nil {
		t.Fatalf("Receive() should succeed even when parser fails, got: %v", err)
	}

	if saved.ParsedData != nil {
		t.Errorf("ParsedData should be nil when parser fails, got %v", saved.ParsedData)
	}
	if saved.Payload == nil {
		t.Error("Payload should still be stored even when parser fails")
	}
}
