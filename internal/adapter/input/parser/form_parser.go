package parser

import (
	"fmt"
	"net/url"
)

// FormParser parses URL-encoded form bodies into map[string]any.
type FormParser struct{}

func NewFormParser() *FormParser { return &FormParser{} }

func (p *FormParser) Type() string { return "FORM" }

func (p *FormParser) Parse(_ string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("form parser: empty body")
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("form parser: %w", err)
	}
	result := make(map[string]any, len(values))
	for k, v := range values {
		if len(v) == 1 {
			result[k] = v[0]
		} else {
			result[k] = v
		}
	}
	return result, nil
}
