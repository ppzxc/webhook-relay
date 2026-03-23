package parser

import (
	"encoding/json"
	"fmt"
)

// JSONParser parses JSON message bodies into map[string]any.
type JSONParser struct{}

func NewJSONParser() *JSONParser { return &JSONParser{} }

func (p *JSONParser) Type() string { return "JSON" }

func (p *JSONParser) Parse(_ string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("json parser: empty body")
	}
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("json parser: %w", err)
	}
	return result, nil
}
