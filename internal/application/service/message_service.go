package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"
	"relaybox/internal/application/port/input"
	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

// Compile-time interface checks
var _ input.ReceiveMessageUseCase = (*MessageService)(nil)
var _ input.GetMessageUseCase = (*MessageService)(nil)
var _ input.ListMessagesUseCase = (*MessageService)(nil)
var _ input.RequeueMessageUseCase = (*MessageService)(nil)

type MessageService struct {
	repo        output.MessageRepository
	queue       output.MessageQueue
	parserTypes map[string]string
	registry    input.ParserRegistry
}

func NewMessageService(
	repo output.MessageRepository,
	queue output.MessageQueue,
	parserTypes map[string]string,
	registry input.ParserRegistry,
) *MessageService {
	return &MessageService{
		repo:        repo,
		queue:       queue,
		parserTypes: parserTypes,
		registry:    registry,
	}
}

func (s *MessageService) GetByID(ctx context.Context, id string) (domain.Message, error) {
	msg, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return domain.Message{}, fmt.Errorf("get by id: %w", err)
	}
	return msg, nil
}

func (s *MessageService) ListByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error) {
	msgs, err := s.repo.FindByInput(ctx, inputID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list by input: %w", err)
	}
	if msgs == nil {
		return []domain.Message{}, nil
	}
	return msgs, nil
}

func (s *MessageService) Requeue(ctx context.Context, messageID string) (domain.Message, error) {
	msg, err := s.repo.FindByID(ctx, messageID)
	if err != nil {
		return domain.Message{}, fmt.Errorf("requeue: find: %w", err)
	}
	if msg.Status != domain.MessageStatusFailed {
		return domain.Message{}, fmt.Errorf("requeue: %w", domain.ErrInvalidTransition)
	}
	if err := s.repo.UpdateDeliveryState(ctx, messageID, domain.MessageStatusPending, 0, time.Time{}); err != nil {
		return domain.Message{}, fmt.Errorf("requeue: update: %w", err)
	}
	msg.Status = domain.MessageStatusPending
	msg.RetryCount = 0
	msg.LastAttemptAt = nil
	if err := s.queue.Enqueue(ctx, msg); err != nil {
		return domain.Message{}, fmt.Errorf("requeue: enqueue: %w", err)
	}
	return msg, nil
}

func (s *MessageService) Receive(ctx context.Context, inputID string, contentType string, body []byte) (string, error) {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	msg := domain.Message{
		ID:        id,
		Version:   1,
		Input:     inputID,
		Payload:   domain.RawPayload(body),
		CreatedAt: time.Now().UTC(),
		Status:    domain.MessageStatusPending,
	}

	// Parse body if a parser is configured for this input type
	if s.parserTypes != nil && s.registry != nil {
		if parserType, ok := s.parserTypes[inputID]; ok && parserType != "" {
			if parser, err := s.registry.Get(parserType); err == nil {
				if parsed, err := parser.Parse(contentType, body); err == nil {
					msg.ParsedData = parsed
				} else {
					slog.Warn("parser failed, storing raw payload only",
						"input", inputID, "parser", parserType, "err", err)
				}
			} else {
				slog.Warn("parser not found, storing raw payload only",
					"input", inputID, "parser", parserType, "err", err)
			}
		}
	}

	if err := s.repo.Save(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: save: %w", err)
	}
	if err := s.queue.Enqueue(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: enqueue: %w", err)
	}
	slog.Info("message received", "messageID", id, "input", inputID)
	return id, nil
}
