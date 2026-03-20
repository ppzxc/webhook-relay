package output

import "webhook-relay/internal/domain"

type SenderRegistry interface {
	// Get 은 AlertSender(named interface)를 반환한다.
	Get(channelType domain.ChannelType) (AlertSender, error)
}
