package parser

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// XMLParser parses XML bodies into a flat map of element names to text content.
type XMLParser struct{}

func NewXMLParser() *XMLParser { return &XMLParser{} }

func (p *XMLParser) Type() string { return "xml" }

// Parse parses XML body into a flat map of element names to text content.
// Limitations: XML attributes are not captured; for repeated element names, last value wins.
func (p *XMLParser) Parse(_ string, body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("xml parser: empty body")
	}

	decoder := xml.NewDecoder(bytes.NewReader(body))
	result := make(map[string]any)
	var currentElement string

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			currentElement = t.Name.Local
		case xml.CharData:
			if currentElement != "" {
				text := string(bytes.TrimSpace([]byte(t)))
				if text != "" {
					result[currentElement] = text
				}
			}
		case xml.EndElement:
			currentElement = ""
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("xml parser: no elements found")
	}
	return result, nil
}
