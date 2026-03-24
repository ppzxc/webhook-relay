package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gws "github.com/gorilla/websocket"
	wsadapter "relaybox/internal/adapter/input/websocket"
)

type mockUseCase struct{ count atomic.Int32 }

func (m *mockUseCase) Receive(_ context.Context, _ string, _ string, _ []byte) (string, error) {
	m.count.Add(1)
	return "test-id", nil
}

func TestWebSocketHandler_ReceiveMessage(t *testing.T) {
	uc := &mockUseCase{}
	handler := wsadapter.NewHandler(uc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeWS(w, r, "beszel")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	conn, _, err := gws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(gws.TextMessage, []byte(`{"host":"srv1"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.Close()
}

func TestWebSocketHandler_CrossOriginRejected(t *testing.T) {
	uc := &mockUseCase{}
	handler := wsadapter.NewHandler(uc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeWS(w, r, "beszel")
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	header := http.Header{}
	header.Set("Origin", "http://evil.example.com")
	_, resp, err := gws.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected cross-origin connection to be rejected")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
}
