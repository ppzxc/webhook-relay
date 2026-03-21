package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"relaybox/internal/application/port/input"
	"relaybox/internal/domain"
)

type Handler struct {
	uc input.ReceiveMessageUseCase
}

func NewHandler(uc input.ReceiveMessageUseCase) *Handler {
	return &Handler{uc: uc}
}

func (h *Handler) PostMessage(w http.ResponseWriter, r *http.Request) {
	inputID := chi.URLParam(r, "inputId")
	inputType := inputTypeFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, r, http.StatusRequestEntityTooLarge, "Payload Too Large", "request body exceeds 1MB limit")
		} else {
			writeError(w, r, http.StatusBadRequest, "Bad Request", "failed to read body")
		}
		return
	}

	messageID, err := h.uc.Receive(r.Context(), inputType, r.Header.Get("Content-Type"), body)
	if err != nil {
		mapError(w, r, err)
		return
	}

	resp := map[string]any{
		"id":        messageID,
		"inputId":   inputID,
		"status":    string(domain.MessageStatusPending),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/inputs/%s/messages/%s", inputID, messageID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
