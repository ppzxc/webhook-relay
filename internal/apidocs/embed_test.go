package apidocs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webhook-relay/internal/apidocs"
)

func TestOpenAPIHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi", nil)
	w := httptest.NewRecorder()
	apidocs.OpenAPIHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("body is empty")
	}
	// placeholder는 version "0.0.0" — 실제 스펙은 "2026-03-20" 이어야 한다
	if !strings.Contains(w.Body.String(), "webhook-relay API") {
		t.Error("openapi.yaml does not contain expected title")
	}
}

func TestAsyncAPIHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs/asyncapi", nil)
	w := httptest.NewRecorder()
	apidocs.AsyncAPIHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("body is empty")
	}
	if !strings.Contains(w.Body.String(), "webhook-relay WebSocket API") {
		t.Error("asyncapi.yaml does not contain expected title")
	}
}

func TestRedocHTMLHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	apidocs.RedocHTMLHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<redoc") {
		t.Error("body does not contain <redoc element")
	}
	if !strings.Contains(body, `spec-url="/docs/openapi"`) {
		t.Error("body does not contain spec-url=/docs/openapi")
	}
}
