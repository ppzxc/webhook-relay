package domain

type MessageStatus string

const (
	MessageStatusPending   MessageStatus = "PENDING"
	MessageStatusDelivered MessageStatus = "DELIVERED"
	MessageStatusFailed    MessageStatus = "FAILED"
)

func (s MessageStatus) IsValid() bool {
	switch s {
	case MessageStatusPending, MessageStatusDelivered, MessageStatusFailed:
		return true
	}
	return false
}

func (s MessageStatus) CanTransitionTo(next MessageStatus) bool {
	switch s {
	case MessageStatusPending:
		return next == MessageStatusDelivered || next == MessageStatusFailed
	case MessageStatusFailed:
		return next == MessageStatusPending
	}
	return false
}
