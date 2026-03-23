package parser

import (
	"fmt"

	"github.com/kr/logfmt"
)

// LogfmtParser parses logfmt-encoded bodies into map[string]any.
type LogfmtParser struct{}

func NewLogfmtParser() *LogfmtParser { return &LogfmtParser{} }

func (p *LogfmtParser) Type() string { return "LOGFMT" }

func (p *LogfmtParser) Parse(_ string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("logfmt parser: empty body")
	}

	result := make(map[string]any)
	handler := logfmt.HandlerFunc(func(key, val []byte) error {
		result[string(key)] = string(val)
		return nil
	})

	if err := logfmt.Unmarshal(body, handler); err != nil {
		return nil, fmt.Errorf("logfmt parser: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("logfmt parser: no key-value pairs found")
	}
	return result, nil
}
