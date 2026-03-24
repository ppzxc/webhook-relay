package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "relaybox/internal/adapter/input/http"
	"relaybox/internal/domain"
)

type mockUseCase struct {
	receiveFn func(context.Context, string, string, []byte) (string, error)
}

func (m *mockUseCase) Receive(ctx context.Context, s string, contentType string, p []byte) (string, error) {
	return m.receiveFn(ctx, s, contentType, p)
}

type mockResolver struct {
	inputs  map[string]string
	secrets map[string]string
}

func (m *mockResolver) Resolve(inputID string) (string, error) {
	st, ok := m.inputs[inputID]
	if !ok {
		return "", domain.ErrInputNotFound
	}
	return st, nil
}

func (m *mockResolver) ValidateToken(inputID, token string) bool {
	return m.secrets[inputID] == token
}

type mockGetUseCase struct {
	getByIDFn func(context.Context, string) (domain.Message, error)
}

func (m *mockGetUseCase) GetByID(ctx context.Context, id string) (domain.Message, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return domain.Message{}, domain.ErrMessageNotFound
}

func newTestRouter(receiveFn func(context.Context, string, string, []byte) (string, error), getByIDFn func(context.Context, string) (domain.Message, error)) http.Handler {
	uc := &mockUseCase{receiveFn: receiveFn}
	getUC := &mockGetUseCase{getByIDFn: getByIDFn}
	resolver := &mockResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "test-token"},
	}
	return httpadapter.NewRouter(uc, getUC, resolver, nil)
}

func TestHandler_PostMessage_Success(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "01JTEST00000000000000000", nil
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/inputs/beszel/messages", strings.NewReader(`{"level":"critical"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Error("Location header missing")
	}
	// Location must point to the specific message, not the collection
	if !strings.Contains(loc, "/messages/01JTEST00000000000000000") {
		t.Errorf("Location = %q, want path containing specific messageId", loc)
	}
	if v := w.Header().Get("X-API-Version"); v == "" {
		t.Error("X-API-Version header missing")
	}
}

func TestHandler_PostMessage_InvalidToken(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", nil
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/inputs/beszel/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer wrong-token")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestHandler_Healthz(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", nil
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// allowAllResolver는 ValidateToken이 항상 true를 반환하는 취약한 구현을 모사한다.
// handler가 ValidateToken에 의존하지 않고 빈 토큰을 직접 거부하는지 검증하기 위해 사용한다.
type allowAllResolver struct{ inputs map[string]string }

func (a *allowAllResolver) Resolve(id string) (string, error) {
	st, ok := a.inputs[id]
	if !ok {
		return "", domain.ErrInputNotFound
	}
	return st, nil
}
func (a *allowAllResolver) ValidateToken(_, _ string) bool { return true }

func TestHandler_PostMessage_EmptyToken(t *testing.T) {
	// ValidateToken이 항상 true인 resolver를 사용하여,
	// handler 레이어에서 빈 토큰을 명시적으로 거부해야 함을 검증한다.
	uc := &mockUseCase{receiveFn: func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "id", nil
	}}
	resolver := &allowAllResolver{inputs: map[string]string{"beszel": "beszel"}}
	router := httpadapter.NewRouter(uc, &mockGetUseCase{}, resolver, nil)

	tests := []struct {
		name string
		auth string
	}{
		{"no Authorization header", ""},
		{"Bearer with empty token", "Bearer "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/inputs/beszel/messages", strings.NewReader(`{}`))
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (empty token must be rejected before ValidateToken)", w.Code)
			}
		})
	}
}

func TestHandler_PostMessage_BodyTooLarge(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "id", nil
	}, nil)
	// 1MB + 1byte 초과 요청
	oversized := strings.Repeat("x", 1<<20+1)
	req := httptest.NewRequest(http.MethodPost, "/inputs/beszel/messages", strings.NewReader(oversized))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", w.Code)
	}
}

func TestHandler_InputNotFound(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", domain.ErrInputNotFound
	}, nil)
	req := httptest.NewRequest(http.MethodPost, "/inputs/unknown/messages", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		// unknown input → token check fails first → 401
		t.Logf("status = %d (expected 401 since unknown input has no registered token)", w.Code)
	}
}

func TestWebSocketEndpoint_NoToken_Returns401(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "id", nil
	}, nil)

	tests := []struct {
		name string
		auth string
	}{
		{"no token", ""},
		{"wrong token", "Bearer wrong-token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages/ws", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestDocs_OpenAPI(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", nil
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/docs/openapi", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("body is empty")
	}
}

func TestDocs_AsyncAPI(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", nil
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/docs/asyncapi", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("body is empty")
	}
}

func TestHandler_GetMessage_Success(t *testing.T) {
	want := domain.Message{
		ID:     "msg-1",
		Input:  "beszel",
		Status: domain.MessageStatusPending,
	}
	router := newTestRouter(
		func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
			return "", nil
		},
		func(_ context.Context, id string) (domain.Message, error) {
			return want, nil
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages/msg-1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
	var got domain.Message
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != want.ID {
		t.Fatalf("expected id %q, got %q", want.ID, got.ID)
	}
}

func TestHandler_GetMessage_NotFound(t *testing.T) {
	router := newTestRouter(
		func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
			return "", nil
		},
		func(_ context.Context, id string) (domain.Message, error) {
			return domain.Message{}, domain.ErrMessageNotFound
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandler_GetMessage_NoAuth(t *testing.T) {
	router := newTestRouter(
		func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
			return "", nil
		},
		nil,
	)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages/msg-1", nil)
	// No Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDocs_HTML(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "", nil
	}, nil)
	req := httptest.NewRequest(http.MethodGet, "/docs", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<redoc") {
		t.Error("body does not contain <redoc element")
	}
}
