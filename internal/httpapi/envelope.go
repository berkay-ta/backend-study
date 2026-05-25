// Package httpapi exposes the service over HTTP: router, handlers, middleware,
// DTOs, and the response envelope. Handlers delegate all business logic to
// app services.
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type Envelope struct {
	Data  any            `json:"data"`
	Meta  Meta           `json:"meta"`
	Error *ErrorEnvelope `json:"error"`
}

type Meta struct {
	RequestID     string `json:"request_id"`
	LeagueVersion *int64 `json:"league_version,omitempty"`
}

type ErrorEnvelope struct {
	Code    apperror.Code         `json:"code"`
	Message string                `json:"message"`
	Fields  []apperror.FieldError `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func marshalData(r *http.Request, data any, leagueVersion *int64) ([]byte, error) {
	body, err := json.Marshal(Envelope{
		Data: data,
		Meta: Meta{RequestID: RequestIDFromContext(r.Context()), LeagueVersion: leagueVersion},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return append(body, '\n'), nil
}

func writeData(w http.ResponseWriter, r *http.Request, status int, data any, leagueVersion *int64) {
	writeJSON(w, status, Envelope{
		Data: data,
		Meta: Meta{RequestID: RequestIDFromContext(r.Context()), LeagueVersion: leagueVersion},
	})
}

func writeCreated(w http.ResponseWriter, r *http.Request, data any, leagueVersion *int64) {
	writeData(w, r, http.StatusCreated, data, leagueVersion)
}

func writeOK(w http.ResponseWriter, r *http.Request, data any, leagueVersion *int64) {
	writeData(w, r, http.StatusOK, data, leagueVersion)
}
