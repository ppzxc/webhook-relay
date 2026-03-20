package input

import (
	"context"

	"relaybox/internal/domain"
)

// ReceiveMessageUseCase 메시지 수신 유스케이스.
// input은 반드시 domain.InputType 값 (예: "BESZEL")으로 전달된다.
// contentType은 HTTP Content-Type 헤더 값이다 (파서 선택에 사용).
// 성공 시 생성된 message ID를 반환한다.
type ReceiveMessageUseCase interface {
	Receive(ctx context.Context, input domain.InputType, contentType string, body []byte) (string, error)
}
