package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type CachedResponse struct {
	Status int
	Body   []byte
}

type IdempotencyService struct {
	tx      TxRunner
	records IdempotencyRepository
}

func NewIdempotencyService(tx TxRunner, records IdempotencyRepository) *IdempotencyService {
	if tx == nil {
		panic("app.NewIdempotencyService: tx must not be nil")
	}
	if records == nil {
		panic("app.NewIdempotencyService: records must not be nil")
	}
	return &IdempotencyService{tx: tx, records: records}
}

func (s *IdempotencyService) Run(
	ctx context.Context,
	key string,
	requestHash string,
	fn func(context.Context) (CachedResponse, error),
) (CachedResponse, bool, error) {
	var (
		out      CachedResponse
		replayed bool
		created  bool
	)
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		rec, ok, err := s.records.Reserve(ctx, key, requestHash)
		if err != nil {
			return err
		}
		created = ok
		if !created {
			if rec.RequestHash != requestHash {
				return apperror.Conflict(
					apperror.CodeIdempotencyKeyConflict,
					"Idempotency-Key was already used for a different request",
				)
			}
			// A committed record is always completed (Reserve and Complete
			// share a tx); anything else means the invariant broke.
			if rec.State != IdempotencyCompleted {
				return apperror.Internal(fmt.Errorf("idempotency key %q in unexpected committed state %q", key, rec.State))
			}
			out = CachedResponse{Status: rec.ResponseStatus, Body: append([]byte(nil), rec.ResponseBody...)}
			replayed = true
			return nil
		}

		// Abort frees the key for retry. The SQL store's tx rollback already
		// undoes the reservation; the in-memory test store doesn't roll back,
		// so it relies on this explicit delete.
		resp, err := fn(ctx)
		if err != nil {
			_ = s.records.Abort(ctx, key)
			return err
		}
		// Only cache a 2xx JSON object; otherwise it's a handler bug.
		if resp.Status < http.StatusOK || resp.Status >= http.StatusMultipleChoices || !jsonObject(resp.Body) {
			_ = s.records.Abort(ctx, key)
			return apperror.Internal(errors.New("invalid idempotent response"))
		}
		if err := s.records.Complete(ctx, key, resp.Status, resp.Body); err != nil {
			return err
		}
		out = CachedResponse{Status: resp.Status, Body: append([]byte(nil), resp.Body...)}
		return nil
	})
	if err != nil {
		return CachedResponse{}, false, err
	}
	return out, replayed, nil
}

func jsonObject(body []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("{"))
}
