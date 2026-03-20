package input

import (
	"context"

	"webhook-relay/internal/domain"
)

// ReceiveAlertUseCase 알람 수신 유스케이스.
// source는 반드시 domain.SourceType 값 (예: "BESZEL")으로 전달된다.
// 성공 시 생성된 alert ID를 반환한다.
type ReceiveAlertUseCase interface {
	Receive(ctx context.Context, source domain.SourceType, payload []byte) (string, error)
}
