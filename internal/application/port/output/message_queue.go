package output

import (
	"context"
	"errors"

	"relaybox/internal/domain"
)

// ErrQueueEmpty is returned by Dequeue when there are no messages in the queue.
var ErrQueueEmpty = errors.New("queue empty")

// AckFunc 전달 성공 후 호출 — 큐에서 영구 삭제
type AckFunc func() error

// NackFunc 전달 실패 후 호출 — 큐에 메시지 반환
type NackFunc func() error

type MessageQueue interface {
	Enqueue(ctx context.Context, msg domain.Message) error
	Dequeue(ctx context.Context) (domain.Message, AckFunc, NackFunc, error)
}
