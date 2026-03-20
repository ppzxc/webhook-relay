package http

import "relaybox/internal/domain"

// InputResolver URL inputID를 domain.InputType으로 변환하고 토큰을 검증한다.
type InputResolver interface {
	Resolve(inputID string) (domain.InputType, error)
	ValidateToken(inputID, token string) bool
}
