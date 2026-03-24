package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	sqliteadapter "relaybox/internal/adapter/output/sqlite"
	"relaybox/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ interface {
	Save(context.Context, domain.Message) error
	FindByID(context.Context, string) (domain.Message, error)
} = (*sqliteadapter.Repository)(nil)

func newTestRepo(t *testing.T) *sqliteadapter.Repository {
	t.Helper()
	repo, err := sqliteadapter.New(":memory:", "")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestRepository_CustomTableName(t *testing.T) {
	dir := t.TempDir()
	repo, err := sqliteadapter.New(filepath.Join(dir, "test.db"), "custom_msgs")
	if err != nil {
		t.Fatalf("New with custom table: %v", err)
	}
	defer repo.Close()

	msg := domain.Message{
		ID:        "01TEST",
		Version:   1,
		Input:     "test-input",
		Payload:   domain.RawPayload(`{"key":"value"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Status:    domain.MessageStatusPending,
	}
	if err := repo.Save(context.Background(), msg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, msg.ID)
	}
}

func TestRepository_SaveAndFindByID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	msg := domain.Message{
		ID: "test-001", Version: 1, Input: "beszel",
		Payload:   domain.RawPayload(`{"host":"srv1"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Status:    domain.MessageStatusPending,
	}
	if err := repo.Save(ctx, msg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	got, err := repo.FindByID(ctx, msg.ID)
	if err != nil {
		t.Fatalf("FindByID() error: %v", err)
	}
	if got.ID != msg.ID || string(got.Payload) != string(msg.Payload) {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestRepository_UpdateDeliveryState(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	msg := domain.Message{ID: "test-002", Version: 1, Input: "dozzle", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending}
	repo.Save(ctx, msg)

	now := time.Now().UTC()
	if err := repo.UpdateDeliveryState(ctx, msg.ID, domain.MessageStatusDelivered, 1, now); err != nil {
		t.Fatalf("UpdateDeliveryState() error: %v", err)
	}
	got, _ := repo.FindByID(ctx, msg.ID)
	if got.Status != domain.MessageStatusDelivered || got.RetryCount != 1 {
		t.Errorf("unexpected state: %+v", got)
	}
}

func TestRepository_FindByInput(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	for _, id := range []string{"a1", "a2", "a3"} {
		repo.Save(ctx, domain.Message{ID: id, Input: "beszel", Payload: domain.RawPayload(`{}`), Status: domain.MessageStatusPending, Version: 1})
	}
	messages, err := repo.FindByInput(ctx, "beszel", 10, 0)
	if err != nil {
		t.Fatalf("FindByInput() error: %v", err)
	}
	if len(messages) != 3 {
		t.Errorf("got %d, want 3", len(messages))
	}
}
