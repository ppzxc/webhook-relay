package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"webhook-relay/internal/domain"
)

type contextKey string

const traceIDKey contextKey = "traceID"

type errorResponse struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Detail  string `json:"detail"`
	TraceID string `json:"traceId,omitempty"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	traceID, _ := r.Context().Value(traceIDKey).(string)
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{
		Type: "/errors/" + http.StatusText(status), Title: title,
		Status: status, Detail: detail, TraceID: traceID,
	})
}

func mapError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidToken):
		writeError(w, r, http.StatusUnauthorized, "Unauthorized", err.Error())
	case errors.Is(err, domain.ErrSourceNotFound):
		writeError(w, r, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, domain.ErrAlertNotFound):
		writeError(w, r, http.StatusNotFound, "Not Found", err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, r, http.StatusUnprocessableEntity, "Unprocessable Entity", err.Error())
	default:
		writeError(w, r, http.StatusInternalServerError, "Internal Server Error", "unexpected error")
	}
}

func apiVersionMiddleware(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

func tokenFromHeader(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

func withTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}
