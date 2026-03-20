package domain

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

type TemplateData struct {
	ID        string
	// Source is intentionally kept as-is; TemplateData will be removed in Phase 4 (CEL/Expr replaces templates)
	Source    string
	Payload   string
	CreatedAt time.Time
}

func RenderTemplate(tmpl string, msg Message) ([]byte, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	data := TemplateData{
		ID:        msg.ID,
		Source:    string(msg.Input),
		Payload:   string(msg.Payload),
		CreatedAt: msg.CreatedAt,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

func ValidateTemplate(tmpl string) error {
	if _, err := template.New("").Parse(tmpl); err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	return nil
}
