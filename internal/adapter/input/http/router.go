package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"relaybox/internal/adapter/input/websocket"
	"relaybox/internal/adapter/input/http/apidocs"
	"relaybox/internal/application/port/input"
)

// API 버전 — X-API-Version 헤더로 반환
const APIVersion = "2026-03-20"

// WSHandler is the subset of websocket.Handler used by the router.
// nil is allowed for tests that don't exercise the /messages/ws path.
type WSHandler interface {
	ServeWS(w http.ResponseWriter, r *http.Request, inputID string)
}

func NewRouter(
	receiveUC input.ReceiveMessageUseCase,
	getUC input.GetMessageUseCase,
	listUC input.ListMessagesUseCase,
	requeueUC input.RequeueMessageUseCase,
	configUC input.ConfigQueryUseCase,
	resolver input.InputResolver,
	ws WSHandler,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(apiVersionMiddleware(APIVersion))

	h := NewHandler(receiveUC, getUC, listUC, requeueUC, configUC)

	r.Get("/healthz", h.Healthz)
	r.Get("/docs", apidocs.RedocHTMLHandler)
	r.Get("/docs/openapi", apidocs.OpenAPIHandler)
	r.Get("/docs/asyncapi", apidocs.AsyncAPIHandler)

	// Config 엔드포인트 — 인증 불필요 (secret 미노출)
	r.Get("/inputs", h.ListInputs)
	r.Get("/outputs", h.ListOutputs)
	r.Get("/outputs/{outputId}", h.GetOutput)

	r.Route("/inputs/{inputId}", func(r chi.Router) {
		// Config 조회 — 인증 불필요
		r.Get("/", h.GetInput)

		// 메시지 엔드포인트 — inputAuthMiddleware 적용
		r.Group(func(r chi.Router) {
			r.Use(inputAuthMiddleware(resolver))
			// 리터럴 /messages/ws를 와일드카드 /messages/{messageId}보다 먼저 등록
			r.Get("/messages/ws", func(w http.ResponseWriter, req *http.Request) {
				if ws == nil {
					writeError(w, req, http.StatusNotImplemented, "Not Implemented", "websocket not configured")
					return
				}
				ws.ServeWS(w, req, inputIDFromContext(req.Context()))
			})
			r.Get("/messages", h.ListMessages)
			r.Post("/messages", h.PostMessage)
			r.Get("/messages/{messageId}", h.GetMessage)
			r.Patch("/messages/{messageId}", h.PatchMessage)
		})
	})

	return r
}

// Ensure *websocket.Handler satisfies WSHandler at compile time.
var _ WSHandler = (*websocket.Handler)(nil)
