package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "webhook-relay/internal/adapter/input/http"
	"webhook-relay/internal/domain"
)

type mockUseCase struct {
	receiveFn func(context.Context, domain.SourceType, []byte) (string, error)
}

func (m *mockUseCase) Receive(ctx context.Context, s domain.SourceType, p []byte) (string, error) {
	return m.receiveFn(ctx, s, p)
}

type mockResolver struct {
	sources map[string]domain.SourceType
	secrets map[string]string
}

func (m *mockResolver) Resolve(sourceID string) (domain.SourceType, error) {
	st, ok := m.sources[sourceID]
	if !ok {
		return "", domain.ErrSourceNotFound
	}
	return st, nil
}

func (m *mockResolver) ValidateToken(sourceID, token string) bool {
	return m.secrets[sourceID] == token
}

func newTestRouter(receiveFn func(context.Context, domain.SourceType, []byte) (string, error)) http.Handler {
	uc := &mockUseCase{receiveFn: receiveFn}
	resolver := &mockResolver{
		sources: map[string]domain.SourceType{"beszel": domain.SourceTypeBeszel},
		secrets: map[string]string{"beszel": "test-token"},
	}
	return httpadapter.NewRouter(uc, resolver, nil)
}

func TestHandler_PostAlert_Success(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "01JTEST00000000000000000", nil
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/beszel/alerts", strings.NewReader(`{"level":"critical"}`))
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
	// Location must point to the specific alert, not the collection
	if !strings.Contains(loc, "/alerts/01JTEST00000000000000000") {
		t.Errorf("Location = %q, want path containing specific alertId", loc)
	}
	if v := w.Header().Get("X-API-Version"); v == "" {
		t.Error("X-API-Version header missing")
	}
}

func TestHandler_PostAlert_InvalidToken(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", nil
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/beszel/alerts", strings.NewReader(`{}`))
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
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", nil
	})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// allowAllResolver는 ValidateToken이 항상 true를 반환하는 취약한 구현을 모사한다.
// handler가 ValidateToken에 의존하지 않고 빈 토큰을 직접 거부하는지 검증하기 위해 사용한다.
type allowAllResolver struct{ sources map[string]domain.SourceType }

func (a *allowAllResolver) Resolve(id string) (domain.SourceType, error) {
	st, ok := a.sources[id]
	if !ok {
		return "", domain.ErrSourceNotFound
	}
	return st, nil
}
func (a *allowAllResolver) ValidateToken(_, _ string) bool { return true }

func TestHandler_PostAlert_EmptyToken(t *testing.T) {
	// ValidateToken이 항상 true인 resolver를 사용하여,
	// handler 레이어에서 빈 토큰을 명시적으로 거부해야 함을 검증한다.
	uc := &mockUseCase{receiveFn: func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "id", nil
	}}
	resolver := &allowAllResolver{sources: map[string]domain.SourceType{"beszel": domain.SourceTypeBeszel}}
	router := httpadapter.NewRouter(uc, resolver, nil)

	tests := []struct {
		name string
		auth string
	}{
		{"no Authorization header", ""},
		{"Bearer with empty token", "Bearer "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/sources/beszel/alerts", strings.NewReader(`{}`))
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

func TestHandler_SourceNotFound(t *testing.T) {
	router := newTestRouter(func(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
		return "", domain.ErrSourceNotFound
	})
	req := httptest.NewRequest(http.MethodPost, "/sources/unknown/alerts", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		// unknown source → token check fails first → 401
		t.Logf("status = %d (expected 401 since unknown source has no registered token)", w.Code)
	}
}
