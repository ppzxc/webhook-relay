package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"webhook-relay/internal/application/port/input"
	"webhook-relay/internal/domain"
)

type Handler struct {
	uc       input.ReceiveAlertUseCase
	resolver SourceResolver
}

func NewHandler(uc input.ReceiveAlertUseCase, resolver SourceResolver) *Handler {
	return &Handler{uc: uc, resolver: resolver}
}

func (h *Handler) PostAlert(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceId")
	token := tokenFromHeader(r)

	if token == "" {
		writeError(w, r, http.StatusUnauthorized, "Unauthorized", "missing or empty bearer token")
		return
	}

	if !h.resolver.ValidateToken(sourceID, token) {
		writeError(w, r, http.StatusUnauthorized, "Unauthorized",
			fmt.Sprintf("invalid or missing token for source: %s", sourceID))
		return
	}

	sourceType, err := h.resolver.Resolve(sourceID)
	if err != nil {
		mapError(w, r, err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "Bad Request", "failed to read body")
		return
	}

	alertID, err := h.uc.Receive(r.Context(), sourceType, body)
	if err != nil {
		mapError(w, r, err)
		return
	}

	resp := map[string]any{
		"id":        alertID,
		"sourceId":  sourceID,
		"status":    string(domain.AlertStatusPending),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/sources/%s/alerts/%s", sourceID, alertID))
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
