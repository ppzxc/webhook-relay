package websocket_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gws "github.com/gorilla/websocket"
	wsadapter "webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/domain"
)

type mockUseCase struct{ count atomic.Int32 }

func (m *mockUseCase) Receive(_ context.Context, _ domain.SourceType, _ []byte) (string, error) {
	m.count.Add(1)
	return "test-id", nil
}

func TestWebSocketHandler_ReceiveMessage(t *testing.T) {
	uc := &mockUseCase{}
	handler := wsadapter.NewHandler(uc)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeWS(w, r, domain.SourceTypeBeszel)
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
