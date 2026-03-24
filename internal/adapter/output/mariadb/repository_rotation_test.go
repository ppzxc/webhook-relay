package mariadb_test

import (
	"context"
	"testing"
	"time"

	"relaybox/internal/domain"
)

func TestRepository_DeleteOlderThan_ByStatus(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)

	msgs := []domain.Message{
		{ID: "old-delivered", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: old, Status: domain.MessageStatusDelivered},
		{ID: "old-failed", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: old, Status: domain.MessageStatusFailed},
		{ID: "old-pending", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: old, Status: domain.MessageStatusPending},
		{ID: "recent-delivered", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: recent, Status: domain.MessageStatusDelivered},
	}
	for _, m := range msgs {
		if err := repo.Save(ctx, m); err != nil {
			t.Fatalf("Save(%s) error: %v", m.ID, err)
		}
	}

	cutoff := now.Add(-24 * time.Hour)
	deleted, err := repo.DeleteOlderThan(ctx, cutoff, []domain.MessageStatus{domain.MessageStatusDelivered, domain.MessageStatusFailed})
	if err != nil {
		t.Fatalf("DeleteOlderThan() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	if _, err := repo.FindByID(ctx, "old-pending"); err != nil {
		t.Errorf("old-pending should still exist: %v", err)
	}
	if _, err := repo.FindByID(ctx, "recent-delivered"); err != nil {
		t.Errorf("recent-delivered should still exist: %v", err)
	}
}

func TestRepository_DeleteOlderThan_AllStatuses(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-48 * time.Hour)

	msgs := []domain.Message{
		{ID: "old-1", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: old, Status: domain.MessageStatusDelivered},
		{ID: "old-2", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: old, Status: domain.MessageStatusPending},
		{ID: "new-1", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: now, Status: domain.MessageStatusDelivered},
	}
	for _, m := range msgs {
		repo.Save(ctx, m)
	}

	deleted, err := repo.DeleteOlderThan(ctx, now.Add(-24*time.Hour), nil)
	if err != nil {
		t.Fatalf("DeleteOlderThan() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	if _, err := repo.FindByID(ctx, "new-1"); err != nil {
		t.Errorf("new-1 should still exist: %v", err)
	}
}

func TestRepository_DeleteOlderThan_CutoffBoundary(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)
	exact := now.Add(-24 * time.Hour)
	before := exact.Add(-time.Second)

	repo.Save(ctx, domain.Message{ID: "exact", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: exact, Status: domain.MessageStatusDelivered})
	repo.Save(ctx, domain.Message{ID: "before", Input: "x", Payload: domain.RawPayload(`{}`), Version: 1, CreatedAt: before, Status: domain.MessageStatusDelivered})

	deleted, err := repo.DeleteOlderThan(ctx, exact, nil)
	if err != nil {
		t.Fatalf("DeleteOlderThan() error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only 'before')", deleted)
	}

	if _, err := repo.FindByID(ctx, "exact"); err != nil {
		t.Errorf("exact-time message should still exist: %v", err)
	}
}
