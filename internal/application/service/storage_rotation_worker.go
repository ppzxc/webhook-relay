package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"relaybox/internal/application/port/output"
)

// StorageRotationWorker periodically deletes old messages from the repository.
type StorageRotationWorker struct {
	repo output.MessageRepository
	cfg  StorageRotationConfig
	wg   sync.WaitGroup
}

// NewStorageRotationWorker creates a new StorageRotationWorker.
func NewStorageRotationWorker(repo output.MessageRepository, cfg StorageRotationConfig) *StorageRotationWorker {
	return &StorageRotationWorker{repo: repo, cfg: cfg}
}

// Start launches the background rotation goroutine.
func (w *StorageRotationWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.loop(ctx)
}

// Wait blocks until the background goroutine exits.
func (w *StorageRotationWorker) Wait() {
	w.wg.Wait()
}

func (w *StorageRotationWorker) loop(ctx context.Context) {
	defer w.wg.Done()

	// 시작 시 즉시 1회 실행
	w.rotate(ctx)

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.rotate(ctx)
		}
	}
}

func (w *StorageRotationWorker) rotate(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-w.cfg.Retention)
	deleted, err := w.repo.DeleteOlderThan(ctx, cutoff, w.cfg.Statuses)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("storage rotation failed", "err", err)
		}
		return
	}
	if deleted > 0 {
		slog.Info("storage rotation complete", "deleted", deleted, "cutoff", cutoff)
	}
	if w.cfg.Hooks.OnRotated != nil {
		w.cfg.Hooks.OnRotated(deleted)
	}
}
