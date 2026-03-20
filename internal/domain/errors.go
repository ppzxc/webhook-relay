package domain

import "errors"

var (
	ErrSourceNotFound    = errors.New("source not found")
	ErrInvalidToken      = errors.New("invalid token")
	ErrAlertNotFound     = errors.New("alert not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrSenderNotFound    = errors.New("sender not registered for channel type")
)
