package sqlite_test

import (
	"context"
	"testing"
	"time"

	sqliteadapter "webhook-relay/internal/adapter/output/sqlite"
	"webhook-relay/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Alert) error
	FindByID(context.Context, string) (domain.Alert, error)
} = (*sqliteadapter.Repository)(nil)

func newTestRepo(t *testing.T) *sqliteadapter.Repository {
	t.Helper()
	repo, err := sqliteadapter.New(":memory:")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestRepository_SaveAndFindByID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	alert := domain.Alert{
		ID: "test-001", Version: 1, Source: domain.SourceTypeBeszel,
		Payload:   domain.RawPayload(`{"host":"srv1"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Status:    domain.AlertStatusPending,
	}
	if err := repo.Save(ctx, alert); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := repo.FindByID(ctx, alert.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if got.ID != alert.ID || string(got.Payload) != string(alert.Payload) {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestRepository_UpdateDeliveryState(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	alert := domain.Alert{ID: "test-002", Version: 1, Source: domain.SourceTypeDozzle, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending}
	repo.Save(ctx, alert)

	now := time.Now().UTC()
	if err := repo.UpdateDeliveryState(ctx, alert.ID, domain.AlertStatusDelivered, 1, now); err != nil {
		t.Fatalf("UpdateDeliveryState() error: %v", err)
	}
	got, _ := repo.FindByID(ctx, alert.ID)
	if got.Status != domain.AlertStatusDelivered || got.RetryCount != 1 {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestRepository_FindBySource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for _, id := range []string{"a1", "a2", "a3"} {
		repo.Save(ctx, domain.Alert{ID: id, Source: domain.SourceTypeBeszel, Payload: domain.RawPayload(`{}`), Status: domain.AlertStatusPending, Version: 1})
	}
	alerts, err := repo.FindBySource(ctx, string(domain.SourceTypeBeszel), 10, 0)
	if err != nil {
		t.Fatalf("FindBySource() error: %v", err)
	}
	if len(alerts) != 3 {
		t.Errorf("got %d, want 3", len(alerts))
	}
}
