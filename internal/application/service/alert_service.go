package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

type AlertService struct {
	repo  output.AlertRepository
	queue output.AlertQueue
}

func NewAlertService(repo output.AlertRepository, queue output.AlertQueue) *AlertService {
	return &AlertService{repo: repo, queue: queue}
}

func (s *AlertService) Receive(ctx context.Context, source domain.SourceType, payload []byte) (string, error) {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	alert := domain.Alert{
		ID:        id,
		Version:   1,
		Source:    source,
		Payload:   domain.RawPayload(payload),
		CreatedAt: time.Now().UTC(),
		Status:    domain.AlertStatusPending,
	}
	if err := s.repo.Save(ctx, alert); err != nil {
		return "", fmt.Errorf("receive: save: %w", err)
	}
	if err := s.queue.Enqueue(ctx, alert); err != nil {
		return "", fmt.Errorf("receive: enqueue: %w", err)
	}
	return id, nil
}
