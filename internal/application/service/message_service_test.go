package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"relaybox/internal/application/port/output"
	"relaybox/internal/application/service"
	"relaybox/internal/domain"
)

type mockRepo struct {
	saveFn   func(context.Context, domain.Message) error
	updateFn func(context.Context, string, domain.MessageStatus, int, time.Time) error
}

func (m *mockRepo) Save(ctx context.Context, a domain.Message) error { return m.saveFn(ctx, a) }
func (m *mockRepo) UpdateDeliveryState(ctx context.Context, id string, s domain.MessageStatus, retry int, t time.Time) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, s, retry, t)
	}
	return nil
}
func (m *mockRepo) FindByID(_ context.Context, _ string) (domain.Message, error) {
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

func TestMessageService_Receive_Success(t *testing.T) {
	var saved domain.Message
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Message) error { saved = a; return nil }}
	var enqueued domain.Message
	queue := &mockQueue{enqueueFn: func(_ context.Context, a domain.Message) error { enqueued = a; return nil }}

	svc := service.NewMessageService(repo, queue)
	id, err := svc.Receive(context.Background(), domain.InputTypeBeszel, []byte(`{"host":"srv1"}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if id == "" {
		t.Error("returned ID should not be empty")
	}
	if saved.Input != domain.InputTypeBeszel {
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

	svc := service.NewMessageService(repo, queue)
	if _, err := svc.Receive(context.Background(), domain.InputTypeBeszel, []byte(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}
