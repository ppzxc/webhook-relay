package output

import "relaybox/internal/domain"

type OutputRegistry interface {
	// Get 은 OutputSender(named interface)를 반환한다.
	Get(outputType domain.OutputType) (OutputSender, error)
}
