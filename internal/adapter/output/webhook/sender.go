package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"relaybox/internal/domain"
)

const defaultTimeoutSec = 10

type Sender struct{}

func NewSender() *Sender { return &Sender{} }

func (s *Sender) Send(ctx context.Context, out domain.Output, msg domain.Message) error {
	body, err := domain.RenderTemplate(out.Template, msg)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	timeoutSec := out.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	if out.SkipTLSVerify {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, out.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if out.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+out.Secret)
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
