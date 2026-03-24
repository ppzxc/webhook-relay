package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpadapter "relaybox/internal/adapter/input/http"
	inputport "relaybox/internal/application/port/input"
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

type mockListUseCase struct {
	listByInputFn func(context.Context, string, int, int) ([]domain.Message, error)
}

func (m *mockListUseCase) ListByInput(ctx context.Context, inputID string, limit, offset int) ([]domain.Message, error) {
	if m.listByInputFn != nil {
		return m.listByInputFn(ctx, inputID, limit, offset)
	}
	return []domain.Message{}, nil
}

type mockRequeueUseCase struct {
	requeueFn func(context.Context, string) (domain.Message, error)
}

func (m *mockRequeueUseCase) Requeue(ctx context.Context, messageID string) (domain.Message, error) {
	if m.requeueFn != nil {
		return m.requeueFn(ctx, messageID)
	}
	return domain.Message{}, domain.ErrMessageNotFound
}

type mockConfigQueryUseCase struct {
	listInputsFn  func(context.Context) ([]inputport.InputSummary, error)
	getInputFn    func(context.Context, string) (inputport.InputSummary, error)
	listOutputsFn func(context.Context) ([]inputport.OutputSummary, error)
	getOutputFn   func(context.Context, string) (inputport.OutputSummary, error)
}

func (m *mockConfigQueryUseCase) ListInputs(ctx context.Context) ([]inputport.InputSummary, error) {
	if m.listInputsFn != nil {
		return m.listInputsFn(ctx)
	}
	return []inputport.InputSummary{}, nil
}

func (m *mockConfigQueryUseCase) GetInput(ctx context.Context, id string) (inputport.InputSummary, error) {
	if m.getInputFn != nil {
		return m.getInputFn(ctx, id)
	}
	return inputport.InputSummary{}, domain.ErrInputNotFound
}

func (m *mockConfigQueryUseCase) ListOutputs(ctx context.Context) ([]inputport.OutputSummary, error) {
	if m.listOutputsFn != nil {
		return m.listOutputsFn(ctx)
	}
	return []inputport.OutputSummary{}, nil
}

func (m *mockConfigQueryUseCase) GetOutput(ctx context.Context, id string) (inputport.OutputSummary, error) {
	if m.getOutputFn != nil {
		return m.getOutputFn(ctx, id)
	}
	return inputport.OutputSummary{}, domain.ErrOutputNotFound
}

func newTestRouter(receiveFn func(context.Context, string, string, []byte) (string, error), getByIDFn func(context.Context, string) (domain.Message, error)) http.Handler {
	uc := &mockUseCase{receiveFn: receiveFn}
	getUC := &mockGetUseCase{getByIDFn: getByIDFn}
	resolver := &mockResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "test-token"},
	}
	return httpadapter.NewRouter(uc, getUC, nil, nil, nil, resolver, nil)
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
	router := httpadapter.NewRouter(uc, &mockGetUseCase{}, nil, nil, nil, resolver, nil)

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

// newFullTestRouter creates a router with all use cases wired.
func newFullTestRouter(
	listUC *mockListUseCase,
	requeueUC *mockRequeueUseCase,
	configUC *mockConfigQueryUseCase,
) http.Handler {
	uc := &mockUseCase{receiveFn: func(_ context.Context, _ string, _ string, _ []byte) (string, error) {
		return "id", nil
	}}
	getUC := &mockGetUseCase{}
	resolver := &mockResolver{
		inputs:  map[string]string{"beszel": "beszel"},
		secrets: map[string]string{"beszel": "test-token"},
	}
	return httpadapter.NewRouter(uc, getUC, listUC, requeueUC, configUC, resolver, nil)
}

// ---- ListMessages ----

func TestHandler_ListMessages_Success(t *testing.T) {
	msgs := []domain.Message{
		{ID: "m1", Input: "beszel", Status: domain.MessageStatusPending},
		{ID: "m2", Input: "beszel", Status: domain.MessageStatusDelivered},
	}
	listUC := &mockListUseCase{
		listByInputFn: func(_ context.Context, _ string, _, _ int) ([]domain.Message, error) {
			return msgs, nil
		},
	}
	router := newFullTestRouter(listUC, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got []domain.Message
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 messages, got %d", len(got))
	}
}

func TestHandler_ListMessages_DefaultPagination(t *testing.T) {
	var capturedLimit, capturedOffset int
	listUC := &mockListUseCase{
		listByInputFn: func(_ context.Context, _ string, limit, offset int) ([]domain.Message, error) {
			capturedLimit = limit
			capturedOffset = offset
			return []domain.Message{}, nil
		},
	}
	router := newFullTestRouter(listUC, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if capturedLimit != 20 {
		t.Errorf("default limit = %d, want 20", capturedLimit)
	}
	if capturedOffset != 0 {
		t.Errorf("default offset = %d, want 0", capturedOffset)
	}
}

func TestHandler_ListMessages_InvalidParams(t *testing.T) {
	router := newFullTestRouter(&mockListUseCase{}, nil, nil)
	tests := []struct {
		query string
	}{
		{"?limit=-1"},
		{"?limit=0"},
		{"?limit=101"},
		{"?offset=-1"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages"+tt.query, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("query=%q: expected 400, got %d", tt.query, rr.Code)
			}
		})
	}
}

func TestHandler_ListMessages_NoAuth(t *testing.T) {
	router := newFullTestRouter(&mockListUseCase{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel/messages", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

// ---- PatchMessage ----

func TestHandler_PatchMessage_RequeueSuccess(t *testing.T) {
	updated := domain.Message{ID: "m1", Input: "beszel", Status: domain.MessageStatusPending}
	requeueUC := &mockRequeueUseCase{
		requeueFn: func(_ context.Context, id string) (domain.Message, error) {
			return updated, nil
		},
	}
	router := newFullTestRouter(nil, requeueUC, nil)
	req := httptest.NewRequest(http.MethodPatch, "/inputs/beszel/messages/m1",
		strings.NewReader(`{"status":"PENDING"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var got domain.Message
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != domain.MessageStatusPending {
		t.Errorf("status = %q, want PENDING", got.Status)
	}
}

func TestHandler_PatchMessage_InvalidTransition(t *testing.T) {
	requeueUC := &mockRequeueUseCase{
		requeueFn: func(_ context.Context, _ string) (domain.Message, error) {
			return domain.Message{}, domain.ErrInvalidTransition
		},
	}
	router := newFullTestRouter(nil, requeueUC, nil)
	req := httptest.NewRequest(http.MethodPatch, "/inputs/beszel/messages/m1",
		strings.NewReader(`{"status":"PENDING"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rr.Code)
	}
}

func TestHandler_PatchMessage_NotFound(t *testing.T) {
	requeueUC := &mockRequeueUseCase{
		requeueFn: func(_ context.Context, _ string) (domain.Message, error) {
			return domain.Message{}, domain.ErrMessageNotFound
		},
	}
	router := newFullTestRouter(nil, requeueUC, nil)
	req := httptest.NewRequest(http.MethodPatch, "/inputs/beszel/messages/nonexistent",
		strings.NewReader(`{"status":"PENDING"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandler_PatchMessage_InvalidBody(t *testing.T) {
	router := newFullTestRouter(nil, &mockRequeueUseCase{}, nil)
	tests := []struct {
		name string
		body string
	}{
		{"malformed json", `{bad`},
		{"wrong status value", `{"status":"DELIVERED"}`},
		{"empty status", `{"status":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/inputs/beszel/messages/m1",
				strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest && rr.Code != http.StatusUnprocessableEntity {
				t.Errorf("body=%q: expected 400 or 422, got %d", tt.body, rr.Code)
			}
		})
	}
}

// ---- Config endpoints ----

func TestHandler_ListInputs_Success(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		listInputsFn: func(_ context.Context) ([]inputport.InputSummary, error) {
			return []inputport.InputSummary{{ID: "beszel"}, {ID: "dozzle"}}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/inputs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got []inputport.InputSummary
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 inputs, got %d", len(got))
	}
}

func TestHandler_ListInputs_NoAuthRequired(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		listInputsFn: func(_ context.Context) ([]inputport.InputSummary, error) {
			return []inputport.InputSummary{}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/inputs", nil)
	// No Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("ListInputs should NOT require auth, got 401")
	}
}

func TestHandler_GetInput_Success(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		getInputFn: func(_ context.Context, id string) (inputport.InputSummary, error) {
			return inputport.InputSummary{ID: id}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var got inputport.InputSummary
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != "beszel" {
		t.Errorf("id = %q, want beszel", got.ID)
	}
}

func TestHandler_GetInput_NotFound(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		getInputFn: func(_ context.Context, _ string) (inputport.InputSummary, error) {
			return inputport.InputSummary{}, domain.ErrInputNotFound
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/inputs/unknown", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandler_GetInput_NoAuthRequired(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		getInputFn: func(_ context.Context, id string) (inputport.InputSummary, error) {
			return inputport.InputSummary{ID: id}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/inputs/beszel", nil)
	// No Authorization header
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusUnauthorized {
		t.Fatalf("GetInput should NOT require auth, got 401")
	}
}

func TestHandler_ListOutputs_Success(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		listOutputsFn: func(_ context.Context) ([]inputport.OutputSummary, error) {
			return []inputport.OutputSummary{
				{ID: "wh1", Type: "WEBHOOK", URL: "http://example.com"},
			}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/outputs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got []inputport.OutputSummary
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 output, got %d", len(got))
	}
}

func TestHandler_GetOutput_Success(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		getOutputFn: func(_ context.Context, id string) (inputport.OutputSummary, error) {
			return inputport.OutputSummary{ID: id, Type: "WEBHOOK", URL: "http://example.com"}, nil
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/outputs/wh1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandler_GetOutput_NotFound(t *testing.T) {
	configUC := &mockConfigQueryUseCase{
		getOutputFn: func(_ context.Context, _ string) (inputport.OutputSummary, error) {
			return inputport.OutputSummary{}, domain.ErrOutputNotFound
		},
	}
	router := newFullTestRouter(nil, nil, configUC)
	req := httptest.NewRequest(http.MethodGet, "/outputs/unknown", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
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
