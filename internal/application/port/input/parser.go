package input

// Parser parses raw message body into structured data.
type Parser interface {
	Parse(contentType string, body []byte) (map[string]any, error)
	Type() string
}

// ParserRegistry provides lookup of parsers by type name.
type ParserRegistry interface {
	Get(parserType string) (Parser, error)
}
