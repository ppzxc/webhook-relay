package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"relaybox/internal/application/service"
	"relaybox/internal/domain"
)

type mockRotationRepo struct {
	deleteOlderThanFn func(ctx context.Context, cutoff time.Time, statuses []domain.MessageStatus) (int64, error)
}

func (m *mockRotationRepo) Save(_ context.Context, _ domain.Message) error          { return nil }
func (m *mockRotationRepo) UpdateDeliveryState(_ context.Context, _ string, _ domain.MessageStatus, _ int, _ time.Time) error {
	return nil
}
func (m *mockRotationRepo) FindByID(_ context.Context, _ string) (domain.Message, error) {
	return domain.Message{}, nil
}
func (m *mockRotationRepo) FindByInput(_ context.Context, _ string, _, _ int) ([]domain.Message, error) {
	return nil, nil
}
func (m *mockRotationRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time, statuses []domain.MessageStatus) (int64, error) {
	if m.deleteOlderThanFn != nil {
		return m.deleteOlderThanFn(ctx, cutoff, statuses)
	}
	return 0, nil
}

func TestStorageRotationWorker_RunsImmediately(t *testing.T) {
	called := make(chan struct{}, 1)
	repo := &mockRotationRepo{
		deleteOlderThanFn: func(_ context.Context, _ time.Time, _ []domain.MessageStatus) (int64, error) {
			select {
			case called <- struct{}{}:
			default:
			}
			return 0, nil
		},
	}

	cfg := service.StorageRotationConfig{
		Retention: 24 * time.Hour,
		Interval:  10 * time.Minute, // 긴 interval — 즉시 실행 후 두 번째 tick 없이 종료
	}
	w := service.NewStorageRotationWorker(repo, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	select {
	case <-called:
		// 즉시 실행 확인
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not run immediately on start")
	}

	cancel()
	w.Wait()
}

func TestStorageRotationWorker_CutoffIsRetentionBefore(t *testing.T) {
	type call struct {
		cutoff   time.Time
		statuses []domain.MessageStatus
	}
	calls := make(chan call, 1)

	statuses := []domain.MessageStatus{domain.MessageStatusDelivered}
	repo := &mockRotationRepo{
		deleteOlderThanFn: func(_ context.Context, cutoff time.Time, s []domain.MessageStatus) (int64, error) {
			select {
			case calls <- call{cutoff: cutoff, statuses: s}:
			default:
			}
			return 5, nil
		},
	}

	retention := 24 * time.Hour
	before := time.Now().UTC()

	cfg := service.StorageRotationConfig{
		Retention: retention,
		Interval:  10 * time.Minute,
		Statuses:  statuses,
	}
	w := service.NewStorageRotationWorker(repo, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	select {
	case c := <-calls:
		after := time.Now().UTC()
		expectedLow := before.Add(-retention)
		expectedHigh := after.Add(-retention)
		if c.cutoff.Before(expectedLow) || c.cutoff.After(expectedHigh) {
			t.Errorf("cutoff %v not in expected range [%v, %v]", c.cutoff, expectedLow, expectedHigh)
		}
		if len(c.statuses) != 1 || c.statuses[0] != domain.MessageStatusDelivered {
			t.Errorf("statuses = %v, want [DELIVERED]", c.statuses)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not call DeleteOlderThan")
	}

	cancel()
	w.Wait()
}

func TestStorageRotationWorker_EmptyStatuses(t *testing.T) {
	var gotStatuses []domain.MessageStatus
	done := make(chan struct{})

	repo := &mockRotationRepo{
		deleteOlderThanFn: func(_ context.Context, _ time.Time, s []domain.MessageStatus) (int64, error) {
			gotStatuses = s
			close(done)
			return 0, nil
		},
	}

	cfg := service.StorageRotationConfig{
		Retention: 24 * time.Hour,
		Interval:  10 * time.Minute,
		Statuses:  nil, // 비어있음 → 모든 상태 삭제
	}
	w := service.NewStorageRotationWorker(repo, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	select {
	case <-done:
		if gotStatuses != nil {
			t.Errorf("statuses = %v, want nil", gotStatuses)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not run")
	}

	cancel()
	w.Wait()
}

func TestStorageRotationWorker_GracefulShutdown(t *testing.T) {
	repo := &mockRotationRepo{}
	cfg := service.StorageRotationConfig{
		Retention: 24 * time.Hour,
		Interval:  10 * time.Minute,
	}
	w := service.NewStorageRotationWorker(repo, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	cancel()

	done := make(chan struct{})
	go func() { w.Wait(); close(done) }()

	select {
	case <-done:
		// graceful shutdown 확인
	case <-time.After(2 * time.Second):
		t.Fatal("Wait() did not return after context cancel")
	}
}

func TestStorageRotationWorker_OnRotatedHook(t *testing.T) {
	var count atomic.Int64

	repo := &mockRotationRepo{
		deleteOlderThanFn: func(_ context.Context, _ time.Time, _ []domain.MessageStatus) (int64, error) {
			return 7, nil
		},
	}

	done := make(chan struct{})
	cfg := service.StorageRotationConfig{
		Retention: 24 * time.Hour,
		Interval:  10 * time.Minute,
		Hooks: service.StorageRotationHooks{
			OnRotated: func(deleted int64) {
				count.Store(deleted)
				close(done)
			},
		},
	}
	w := service.NewStorageRotationWorker(repo, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	select {
	case <-done:
		if count.Load() != 7 {
			t.Errorf("OnRotated called with %d, want 7", count.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnRotated hook was not called")
	}

	cancel()
	w.Wait()
}
