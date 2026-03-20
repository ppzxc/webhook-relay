package domain

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

type TemplateData struct {
	ID        string
	Source    string
	Payload   string
	CreatedAt time.Time
}

func RenderTemplate(tmpl string, alert Alert) ([]byte, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	data := TemplateData{
		ID:        alert.ID,
		Source:    string(alert.Source),
		Payload:   string(alert.Payload),
		CreatedAt: alert.CreatedAt,
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
