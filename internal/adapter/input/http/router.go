package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"relaybox/internal/adapter/input/websocket"
	"relaybox/internal/apidocs"
	"relaybox/internal/application/port/input"
	"relaybox/internal/domain"
)

// API 버전 — X-API-Version 헤더로 반환
const APIVersion = "2026-03-20"

// WSHandler is the subset of websocket.Handler used by the router.
// nil is allowed for tests that don't exercise the /messages/ws path.
type WSHandler interface {
	ServeWS(w http.ResponseWriter, r *http.Request, inputType domain.InputType)
}

func NewRouter(uc input.ReceiveMessageUseCase, resolver input.InputResolver, ws WSHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(apiVersionMiddleware(APIVersion))

	h := NewHandler(uc)

	r.Get("/healthz", h.Healthz)
	r.Get("/docs", apidocs.RedocHTMLHandler)
	r.Get("/docs/openapi", apidocs.OpenAPIHandler)
	r.Get("/docs/asyncapi", apidocs.AsyncAPIHandler)

	r.Route("/inputs/{inputId}", func(r chi.Router) {
		r.Use(inputAuthMiddleware(resolver))
		// 리터럴 /messages/ws를 와일드카드 /messages/{messageId}보다 먼저 등록
		r.Get("/messages/ws", func(w http.ResponseWriter, req *http.Request) {
			if ws == nil {
				writeError(w, req, http.StatusNotImplemented, "Not Implemented", "websocket not configured")
				return
			}
			ws.ServeWS(w, req, inputTypeFromContext(req.Context()))
		})
		r.Post("/messages", h.PostMessage)
		r.Get("/messages/{messageId}", func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, http.StatusNotImplemented, "Not Implemented",
				"get message by ID is not yet implemented")
		})
	})

	return r
}

// Ensure *websocket.Handler satisfies WSHandler at compile time.
var _ WSHandler = (*websocket.Handler)(nil)
