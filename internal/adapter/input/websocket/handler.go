package websocket

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type Handler struct{ uc input.ReceiveAlertUseCase }

func NewHandler(uc input.ReceiveAlertUseCase) *Handler { return &Handler{uc: uc} }

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request, source domain.SourceType) {
	conn, err := upgrader.Upgrade(w, r, nil)
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
