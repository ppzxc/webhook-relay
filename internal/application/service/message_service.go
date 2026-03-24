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

type MessageService struct {
	repo        output.MessageRepository
	queue       output.MessageQueue
	parserTypes map[domain.InputType]string
	registry    input.ParserRegistry
}

func NewMessageService(
	repo output.MessageRepository,
	queue output.MessageQueue,
	parserTypes map[domain.InputType]string,
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

func (s *MessageService) Receive(ctx context.Context, inputType domain.InputType, contentType string, body []byte) (string, error) {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String()
	msg := domain.Message{
		ID:        id,
		Version:   1,
		Input:     inputType,
		Payload:   domain.RawPayload(body),
		CreatedAt: time.Now().UTC(),
		Status:    domain.MessageStatusPending,
	}

	// Parse body if a parser is configured for this input type
	if s.parserTypes != nil && s.registry != nil {
		if parserType, ok := s.parserTypes[inputType]; ok && parserType != "" {
			if parser, err := s.registry.Get(parserType); err == nil {
				if parsed, err := parser.Parse(contentType, body); err == nil {
					msg.ParsedData = parsed
				} else {
					slog.Warn("parser failed, storing raw payload only",
						"input", inputType, "parser", parserType, "err", err)
				}
			} else {
				slog.Warn("parser not found, storing raw payload only",
					"input", inputType, "parser", parserType, "err", err)
			}
		}
	}

	if err := s.repo.Save(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: save: %w", err)
	}
	if err := s.queue.Enqueue(ctx, msg); err != nil {
		return "", fmt.Errorf("receive: enqueue: %w", err)
	}
	return id, nil
}
