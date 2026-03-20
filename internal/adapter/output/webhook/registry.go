package webhook

import (
	"fmt"

	"webhook-relay/internal/application/port/output"
	"webhook-relay/internal/domain"
)

type Registry struct {
	senders map[domain.ChannelType]output.AlertSender
}

func NewRegistry(senders map[domain.ChannelType]output.AlertSender) *Registry {
	return &Registry{senders: senders}
}

func (r *Registry) Get(t domain.ChannelType) (output.AlertSender, error) {
	s, ok := r.senders[t]
	if !ok {
		return nil, fmt.Errorf("get sender %q: %w", t, domain.ErrSenderNotFound)
	}
	return s, nil
}
