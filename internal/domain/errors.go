package domain

import "errors"

var (
	ErrInputNotFound        = errors.New("input not found")
	ErrInvalidToken         = errors.New("invalid token")
	ErrMessageNotFound      = errors.New("message not found")
	ErrInvalidTransition    = errors.New("invalid status transition")
	ErrOutputSenderNotFound = errors.New("sender not registered for output type")
	ErrOutputNotFound       = errors.New("output not found")
)
