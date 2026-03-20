package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

type MessageService struct {
	repo  output.MessageRepository
	queue output.MessageQueue
}

func NewMessageService(repo output.MessageRepository, queue output.MessageQueue) *MessageService {
	return &MessageService{repo: repo, queue: queue}
}

func (s *MessageService) Receive(ctx context.Context, inputType domain.InputType, payload []byte) (string, error) {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	msg := domain.Message{
		ID:        id,
		Version:   1,
		Input:     inputType,
		Payload:   domain.RawPayload(payload),
		CreatedAt: time.Now().UTC(),
		Status:    domain.MessageStatusPending,
	}
	if err := s.repo.Save(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: save: %w", err)
	}
	if err := s.queue.Enqueue(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: enqueue: %w", err)
	}
	return id, nil
}
