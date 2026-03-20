package domain

type AlertStatus string

const (
	AlertStatusPending   AlertStatus = "PENDING"
	AlertStatusDelivered AlertStatus = "DELIVERED"
	AlertStatusFailed    AlertStatus = "FAILED"
)

func (s AlertStatus) IsValid() bool {
	switch s {
	case AlertStatusPending, AlertStatusDelivered, AlertStatusFailed:
		return true
	}
	return false
}

func (s AlertStatus) CanTransitionTo(next AlertStatus) bool {
	switch s {
	case AlertStatusPending:
		return next == AlertStatusDelivered || next == AlertStatusFailed
	case AlertStatusFailed:
		return next == AlertStatusPending
	}
	return false
}
