package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// sqlNoRows is exported via this file for use across the package's tests.
func sqlNoRows() error { return sql.ErrNoRows }

func TestWithTx_Commit(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectCommit()

	called := false
	err := store.WithTx(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if !called {
		t.Error("expected callback to be called")
	}
	assertExpectations(t, mock)
}

func TestWithTx_NestedReusesOuterTransaction(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectCommit()

	called := false
	err := store.WithTx(context.Background(), func(ctx context.Context) error {
		return store.WithTx(ctx, func(ctx context.Context) error {
			called = true
			return nil
		})
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if !called {
		t.Error("expected nested callback to be called")
	}
	assertExpectations(t, mock)
}

func TestWithTx_RollbackOnError(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectRollback()

	sentinel := errors.New("boom")
	err := store.WithTx(context.Background(), func(ctx context.Context) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel wrapped, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestWithTx_RollbackOnPanic(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectBegin()
	mock.ExpectRollback()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected re-panic, got none")
		}
		assertExpectations(t, mock)
	}()

	_ = store.WithTx(context.Background(), func(ctx context.Context) error {
		panic("kaboom")
	})
}
