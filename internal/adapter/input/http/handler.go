package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"relaybox/internal/application/port/input"
	"relaybox/internal/domain"
)

type Handler struct {
	receiveUC input.ReceiveMessageUseCase
	getUC     input.GetMessageUseCase
	listUC    input.ListMessagesUseCase
	requeueUC input.RequeueMessageUseCase
	configUC  input.ConfigQueryUseCase
}

func NewHandler(
	receiveUC input.ReceiveMessageUseCase,
	getUC input.GetMessageUseCase,
	listUC input.ListMessagesUseCase,
	requeueUC input.RequeueMessageUseCase,
	configUC input.ConfigQueryUseCase,
) *Handler {
	return &Handler{
		receiveUC: receiveUC,
		getUC:     getUC,
		listUC:    listUC,
		requeueUC: requeueUC,
		configUC:  configUC,
	}
}

func (h *Handler) PostMessage(w http.ResponseWriter, r *http.Request) {
	inputID := chi.URLParam(r, "inputId")
	resolvedInputID := inputIDFromContext(r.Context())

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

	messageID, err := h.receiveUC.Receive(r.Context(), resolvedInputID, r.Header.Get("Content-Type"), body)
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

func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")
	msg, err := h.getUC.GetByID(r.Context(), messageID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(msg)
}

func (h *Handler) ListMessages(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := parsePagination(w, r)
	if !ok {
		return
	}
	inputID := inputIDFromContext(r.Context())
	msgs, err := h.listUC.ListByInput(r.Context(), inputID, limit, offset)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(msgs)
}

func (h *Handler) PatchMessage(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageId")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, r, http.StatusBadRequest, "Bad Request", "invalid request body")
		return
	}
	if body.Status != string(domain.MessageStatusPending) {
		writeError(w, r, http.StatusUnprocessableEntity, "Unprocessable Entity",
			fmt.Sprintf("only status=%q is accepted", domain.MessageStatusPending))
		return
	}

	msg, err := h.requeueUC.Requeue(r.Context(), messageID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(msg)
}

func (h *Handler) ListInputs(w http.ResponseWriter, r *http.Request) {
	inputs, err := h.configUC.ListInputs(r.Context())
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(inputs)
}

func (h *Handler) GetInput(w http.ResponseWriter, r *http.Request) {
	inputID := chi.URLParam(r, "inputId")
	inp, err := h.configUC.GetInput(r.Context(), inputID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(inp)
}

func (h *Handler) ListOutputs(w http.ResponseWriter, r *http.Request) {
	outputs, err := h.configUC.ListOutputs(r.Context())
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(outputs)
}

func (h *Handler) GetOutput(w http.ResponseWriter, r *http.Request) {
	outputID := chi.URLParam(r, "outputId")
	out, err := h.configUC.GetOutput(r.Context(), outputID)
	if err != nil {
		mapError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(out)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// parsePagination parses limit and offset query params.
// limit: 1-100, default 20. offset: >= 0, default 0.
// Returns false and writes an error response if params are invalid.
func parsePagination(w http.ResponseWriter, r *http.Request) (limit, offset int, ok bool) {
	limit = 20
	offset = 0

	if s := r.URL.Query().Get("limit"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 100 {
			writeError(w, r, http.StatusBadRequest, "Bad Request", "limit must be between 1 and 100")
			return 0, 0, false
		}
		limit = v
	}
	if s := r.URL.Query().Get("offset"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			writeError(w, r, http.StatusBadRequest, "Bad Request", "offset must be >= 0")
			return 0, 0, false
		}
		offset = v
	}
	return limit, offset, true
}
