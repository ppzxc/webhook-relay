package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"webhook-relay/internal/adapter/input/websocket"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

// API 버전 — X-API-Version 헤더로 반환
const APIVersion = "2026-03-20"

// WSHandler is the subset of websocket.Handler used by the router.
// nil is allowed for tests that don't exercise the /alerts/ws path.
type WSHandler interface {
	ServeWS(w http.ResponseWriter, r *http.Request, source domain.SourceType)
}

func NewRouter(uc input.ReceiveAlertUseCase, resolver SourceResolver, ws WSHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(apiVersionMiddleware(APIVersion))

	h := NewHandler(uc, resolver)

	r.Get("/healthz", h.Healthz)

	r.Route("/sources/{sourceId}", func(r chi.Router) {
		// 리터럴 /alerts/ws를 와일드카드 /alerts/{alertId}보다 먼저 등록
		r.Get("/alerts/ws", func(w http.ResponseWriter, req *http.Request) {
			sourceID := chi.URLParam(req, "sourceId")
			token := tokenFromHeader(req)
			if token == "" || !resolver.ValidateToken(sourceID, token) {
				writeError(w, req, http.StatusUnauthorized, "Unauthorized", "invalid or missing token")
				return
			}
			sourceType, err := resolver.Resolve(sourceID)
			if err != nil {
				writeError(w, req, http.StatusUnauthorized, "Unauthorized", "unknown source")
				return
			}
			if ws == nil {
				writeError(w, req, http.StatusNotImplemented, "Not Implemented", "websocket not configured")
				return
			}
			ws.ServeWS(w, req, sourceType)
		})
		r.Post("/alerts", h.PostAlert)
		r.Get("/alerts/{alertId}", h.Healthz) // placeholder
	})

	return r
}

// Ensure *websocket.Handler satisfies WSHandler at compile time.
var _ WSHandler = (*websocket.Handler)(nil)
