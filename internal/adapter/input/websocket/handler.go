package websocket

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

type Handler struct {
	uc       input.ReceiveAlertUseCase
	upgrader websocket.Upgrader
}

func NewHandler(uc input.ReceiveAlertUseCase) *Handler {
	return &Handler{
		uc:       uc,
		upgrader: websocket.Upgrader{CheckOrigin: sameHostOrigin},
	}
}

// sameHostOrigin은 Origin 헤더가 없거나(비브라우저 클라이언트) 서버와 동일 호스트인 경우만 허용한다.
// Origin이 다른 도메인이면 CSWSH(Cross-Site WebSocket Hijacking)을 방지하기 위해 거부한다.
func sameHostOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request, source domain.SourceType) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("ws upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("ws read error", "source", source, "err", err)
			}
			return
		}
		if _, err := h.uc.Receive(r.Context(), source, msg); err != nil {
			slog.Warn("receive via ws failed", "source", source, "err", err)
		}
	}
}
