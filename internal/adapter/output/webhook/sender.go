package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"webhook-relay/internal/domain"
)

const defaultTimeoutSec = 10

type Sender struct{}

func NewSender() *Sender { return &Sender{} }

func (s *Sender) Send(ctx context.Context, ch domain.Channel, alert domain.Alert) error {
	body, err := domain.RenderTemplate(ch.Template, alert)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	timeoutSec := ch.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	if ch.SkipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ch.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+ch.Secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
