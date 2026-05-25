package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

const (
	idempotencyKeyHeader = "Idempotency-Key"
	maxIdempotencyKeyLen = 120
)

type replayableHandler func(*http.Request) (int, []byte, error)

func requireIdempotency(idem *app.IdempotencyService, next replayableHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if idem == nil {
			writeError(w, r, apperror.Internal(errors.New("idempotency service not configured")))
			return
		}
		key, err := idempotencyKey(r)
		if err != nil {
			writeError(w, r, err)
			return
		}
		hash, err := idempotencyRequestHash(r)
		if err != nil {
			writeError(w, r, err)
			return
		}

		resp, replayed, err := idem.Run(r.Context(), key, hash, func(ctx context.Context) (app.CachedResponse, error) {
			status, body, err := next(r.WithContext(ctx))
			if err != nil {
				return app.CachedResponse{}, err
			}
			return app.CachedResponse{Status: status, Body: body}, nil
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
		if replayed {
			w.Header().Set("X-Idempotent-Replay", "true")
		}
		writeJSONBytes(w, resp.Status, resp.Body)
	}
}

func idempotencyKey(r *http.Request) (string, error) {
	key := strings.TrimSpace(r.Header.Get(idempotencyKeyHeader))
	switch {
	case key == "":
		return "", apperror.New(http.StatusBadRequest,
			apperror.CodeIdempotencyKeyRequired, "Idempotency-Key header is required")
	case len(key) > maxIdempotencyKeyLen:
		return "", apperror.BadRequest("Idempotency-Key header is too long")
	default:
		return key, nil
	}
}

func idempotencyRequestHash(r *http.Request) (string, error) {
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		if err != nil {
			if tooLarge := requestBodyTooLargeError(err); tooLarge != nil {
				return "", tooLarge
			}
			return "", apperror.BadRequest("could not read request body")
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	h := sha256.New()
	_, _ = h.Write([]byte(r.Method))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(r.URL.RequestURI()))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil)), nil
}
