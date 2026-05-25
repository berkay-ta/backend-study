package app_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

func TestIdempotencyService_ReplaysCompletedResponse(t *testing.T) {
	repo := &scriptedIdempotencyRepo{
		record: app.IdempotencyRecord{
			Key:            "k",
			RequestHash:    "h",
			State:          app.IdempotencyCompleted,
			ResponseStatus: http.StatusOK,
			ResponseBody:   []byte(`{"ok":true}`),
		},
	}
	svc := app.NewIdempotencyService(noopTx{}, repo)

	called := false
	resp, replayed, err := svc.Run(context.Background(), "k", "h", func(context.Context) (app.CachedResponse, error) {
		called = true
		return app.CachedResponse{}, nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !replayed || called {
		t.Fatalf("expected replay without calling function")
	}
	if resp.Status != http.StatusOK || string(resp.Body) != `{"ok":true}` {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestIdempotencyService_RejectsDifferentRequestHash(t *testing.T) {
	repo := &scriptedIdempotencyRepo{
		record: app.IdempotencyRecord{
			Key:         "k",
			RequestHash: "old",
			State:       app.IdempotencyCompleted,
		},
	}
	svc := app.NewIdempotencyService(noopTx{}, repo)

	_, _, err := svc.Run(context.Background(), "k", "new", func(context.Context) (app.CachedResponse, error) {
		t.Fatal("function should not be called")
		return app.CachedResponse{}, nil
	})
	ae, ok := apperror.As(err)
	if !ok || ae.Code != apperror.CodeIdempotencyKeyConflict {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestIdempotencyService_UnexpectedCommittedStateIsInternal(t *testing.T) {
	repo := &scriptedIdempotencyRepo{
		record: app.IdempotencyRecord{
			Key:         "k",
			RequestHash: "h",
			State:       app.IdempotencyInProgress,
		},
	}
	svc := app.NewIdempotencyService(noopTx{}, repo)

	_, _, err := svc.Run(context.Background(), "k", "h", func(context.Context) (app.CachedResponse, error) {
		t.Fatal("function should not be called")
		return app.CachedResponse{}, nil
	})
	ae, ok := apperror.As(err)
	if !ok || ae.Code != apperror.CodeInternal {
		t.Fatalf("expected internal error, got %v", err)
	}
}

func TestIdempotencyService_CompletesNewKey(t *testing.T) {
	repo := &scriptedIdempotencyRepo{created: true}
	svc := app.NewIdempotencyService(noopTx{}, repo)

	resp, replayed, err := svc.Run(context.Background(), "k", "h", func(context.Context) (app.CachedResponse, error) {
		return app.CachedResponse{Status: http.StatusOK, Body: []byte(`{"ok":true}`)}, nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if replayed {
		t.Fatal("new key should not be replayed")
	}
	if resp.Status != http.StatusOK || !repo.completed {
		t.Fatalf("expected completed response, resp=%+v completed=%v", resp, repo.completed)
	}
}

type noopTx struct{}

func (noopTx) WithTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type scriptedIdempotencyRepo struct {
	record    app.IdempotencyRecord
	created   bool
	completed bool
}

func (r *scriptedIdempotencyRepo) Reserve(ctx context.Context, key string, requestHash string) (app.IdempotencyRecord, bool, error) {
	if r.created {
		return app.IdempotencyRecord{Key: key, RequestHash: requestHash, State: app.IdempotencyInProgress}, true, nil
	}
	return r.record, false, nil
}

func (r *scriptedIdempotencyRepo) Complete(ctx context.Context, key string, status int, body []byte) error {
	r.completed = true
	r.record = app.IdempotencyRecord{
		Key:            key,
		State:          app.IdempotencyCompleted,
		ResponseStatus: status,
		ResponseBody:   append([]byte(nil), body...),
	}
	return nil
}

func (r *scriptedIdempotencyRepo) Abort(ctx context.Context, key string) error {
	return nil
}
