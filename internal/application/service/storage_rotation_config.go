package service

import (
	"time"

	"relaybox/internal/domain"
)

// StorageRotationHooks contains optional lifecycle callbacks for testing.
type StorageRotationHooks struct {
	// OnRotated is called after each successful rotation with the number of deleted rows.
	OnRotated func(deleted int64)
}

// StorageRotationConfig holds tunable parameters for StorageRotationWorker.
type StorageRotationConfig struct {
	Retention time.Duration
	Interval  time.Duration
	Statuses  []domain.MessageStatus
	Hooks     StorageRotationHooks
}
