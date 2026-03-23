package parser

import (
	"fmt"
	"regexp"
)

// RegexParser parses message bodies using a compiled regex with named capture groups.
type RegexParser struct {
	pattern *regexp.Regexp
}

func NewRegexParser(pattern string) (*RegexParser, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regex parser: compile: %w", err)
	}
	return &RegexParser{pattern: re}, nil
}

func (p *RegexParser) Type() string { return "REGEX" }

func (p *RegexParser) Parse(_ string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("regex parser: empty body")
	}

	match := p.pattern.FindSubmatch(body)
	if match == nil {
		return nil, fmt.Errorf("regex parser: no match")
	}

	result := make(map[string]any)
	for i, name := range p.pattern.SubexpNames() {
		if i != 0 && name != "" && match[i] != nil {
			result[name] = string(match[i])
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("regex parser: no named capture groups in pattern")
	}
	return result, nil
}
