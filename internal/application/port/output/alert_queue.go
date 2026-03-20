package output

import (
	"context"

	"webhook-relay/internal/domain"
)

// AckFunc 전달 성공 후 호출 — 큐에서 영구 삭제
type AckFunc func() error

// NackFunc 전달 실패 후 호출 — 큐에 메시지 반환
type NackFunc func() error

type AlertQueue interface {
	Enqueue(ctx context.Context, alert domain.Alert) error
	Dequeue(ctx context.Context) (domain.Alert, AckFunc, NackFunc, error)
}
