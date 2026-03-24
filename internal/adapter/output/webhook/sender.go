package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"relaybox/internal/domain"
)

const defaultTimeoutSec = 10

type Sender struct {
	transport         *http.Transport
	insecureTransport *http.Transport
}

func NewSender() *Sender {
	base := http.DefaultTransport.(*http.Transport).Clone()
	insecure := http.DefaultTransport.(*http.Transport).Clone()
	insecure.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	return &Sender{transport: base, insecureTransport: insecure}
}

func (s *Sender) Send(ctx context.Context, out domain.Output, payload []byte) error {
	timeoutSec := out.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeoutSec
	}
	t := s.transport
	if out.SkipTLSVerify {
		t = s.insecureTransport
	}
	client := &http.Client{Transport: t, Timeout: time.Duration(timeoutSec) * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, out.URL, bytes.NewReader(payload))
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
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
