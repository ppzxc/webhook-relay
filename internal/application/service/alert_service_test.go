package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/application/service"
	"webhook-relay/internal/domain"
)

type mockRepo struct {
	saveFn func(context.Context, domain.Alert) error
}

func (m *mockRepo) Save(ctx context.Context, a domain.Alert) error { return m.saveFn(ctx, a) }
func (m *mockRepo) UpdateDeliveryState(_ context.Context, _ string, _ domain.AlertStatus, _ int, _ time.Time) error {
	return nil
}
func (m *mockRepo) FindByID(_ context.Context, _ string) (domain.Alert, error) {
	return domain.Alert{}, nil
}
func (m *mockRepo) FindBySource(_ context.Context, _ string, _, _ int) ([]domain.Alert, error) {
	return nil, nil
}

type mockQueue struct {
	enqueueFn func(context.Context, domain.Alert) error
}

func (m *mockQueue) Enqueue(ctx context.Context, a domain.Alert) error { return m.enqueueFn(ctx, a) }
func (m *mockQueue) Dequeue(_ context.Context) (domain.Alert, output.AckFunc, output.NackFunc, error) {
	return domain.Alert{}, nil, nil, nil
}

func TestAlertService_Receive_Success(t *testing.T) {
	var saved domain.Alert
	repo := &mockRepo{saveFn: func(_ context.Context, a domain.Alert) error { saved = a; return nil }}
	var enqueued domain.Alert
	queue := &mockQueue{enqueueFn: func(_ context.Context, a domain.Alert) error { enqueued = a; return nil }}

	svc := service.NewAlertService(repo, queue)
	id, err := svc.Receive(context.Background(), domain.SourceTypeBeszel, []byte(`{"host":"srv1"}`))
	if err != nil {
		t.Fatalf("Receive() error: %v", err)
	}
	if id == "" {
		t.Error("returned ID should not be empty")
	}
	if saved.Source != domain.SourceTypeBeszel {
		t.Errorf("source = %q, want BESZEL", saved.Source)
	}
	if saved.Status != domain.AlertStatusPending {
		t.Errorf("status = %q, want PENDING", saved.Status)
	}
	if saved.ID != id {
		t.Errorf("saved.ID = %q, want returned ID %q", saved.ID, id)
	}
	if enqueued.ID != saved.ID {
		t.Errorf("enqueued ID != saved ID")
	}
}

func TestAlertService_Receive_SaveError(t *testing.T) {
	repo := &mockRepo{saveFn: func(_ context.Context, _ domain.Alert) error { return errors.New("db err") }}
	queue := &mockQueue{enqueueFn: func(_ context.Context, _ domain.Alert) error { return nil }}

	svc := service.NewAlertService(repo, queue)
	if _, err := svc.Receive(context.Background(), domain.SourceTypeBeszel, []byte(`{}`)); err == nil {
		t.Fatal("expected error")
	}
}
