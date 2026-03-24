package mariadb_test

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmariadb "github.com/testcontainers/testcontainers-go/modules/mariadb"

	mariadbadapter "relaybox/internal/adapter/output/mariadb"
	output "relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

// 컴파일 타임 인터페이스 검증
var _ output.MessageRepository = (*mariadbadapter.Repository)(nil)

func newTestRepo(t *testing.T) *mariadbadapter.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping MariaDB integration test (requires Docker)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	ctr, err := tcmariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start MariaDB container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	repo, err := mariadbadapter.New(mariadbadapter.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func newTestRepoWithTableName(t *testing.T, tableName string) *mariadbadapter.Repository {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping MariaDB integration test (requires Docker)")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := context.Background()
	ctr, err := tcmariadb.Run(ctx, "mariadb:11")
	if err != nil {
		t.Fatalf("start MariaDB container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	repo, err := mariadbadapter.New(mariadbadapter.Config{DSN: dsn, TableName: tableName})
	if err != nil {
		t.Fatalf("New() with tableName=%q error: %v", tableName, err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}

func TestRepository_CustomTableName(t *testing.T) {
	repo := newTestRepoWithTableName(t, "custom_msgs")
	ctx := context.Background()

	msg := domain.Message{
		ID:        "01TEST",
		Version:   1,
		Input:     "test-input",
		Payload:   domain.RawPayload(`{"key":"value"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		Status:    domain.MessageStatusPending,
	}
	if err := repo.Save(ctx, msg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, msg.ID)
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
		ID:        "test-001",
		Version:   1,
		Input:     "beszel",
		Payload:   domain.RawPayload(`{"host":"srv1"}`),
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
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

	msg := domain.Message{
		ID:      "test-002",
		Version: 1,
		Input:   "dozzle",
		Payload: domain.RawPayload(`{}`),
		Status:  domain.MessageStatusPending,
	}
	if err := repo.Save(ctx, msg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

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
		if err := repo.Save(ctx, domain.Message{
			ID:      id,
			Input:   "beszel",
			Payload: domain.RawPayload(`{}`),
			Status:  domain.MessageStatusPending,
			Version: 1,
		}); err != nil {
			t.Fatalf("Save() error: %v", err)
		}
	}
	messages, err := repo.FindByInput(ctx, "beszel", 10, 0)
	if err != nil {
		t.Fatalf("FindByInput() error: %v", err)
	}
	if len(messages) != 3 {
		t.Errorf("got %d, want 3", len(messages))
	}
}
