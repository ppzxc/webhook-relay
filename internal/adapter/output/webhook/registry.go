package webhook

import (
	"fmt"

	"relaybox/internal/application/port/output"
	"relaybox/internal/domain"
)

type Registry struct {
	senders map[domain.OutputType]output.OutputSender
}

func NewRegistry(senders map[domain.OutputType]output.OutputSender) *Registry {
	return &Registry{senders: senders}
}

func (r *Registry) Get(t domain.OutputType) (output.OutputSender, error) {
	s, ok := r.senders[t]
	if !ok {
		return nil, fmt.Errorf("get sender %q: %w", t, domain.ErrOutputSenderNotFound)
	}
	return s, nil
}
