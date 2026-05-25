package mysql

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"github.com/iberkayC/case1back/internal/app"
)

const selectIdempotencyByKey = `SELECT idempotency_key, request_hash, state, response_status, response_body
		   FROM idempotency_keys
		  WHERE idempotency_key = ?`

func TestIdempotencyRepo_ReserveCreatesInProgress(t *testing.T) {
	store, mock := newTestStore(t)
	// Insert-first: a fresh key wins the INSERT and owns the reservation; no
	// SELECT happens on this path.
	mock.ExpectExec(`INSERT INTO idempotency_keys (idempotency_key, request_hash, state)
		  VALUES (?, ?, ?)`).
		WithArgs("k", "h", string(app.IdempotencyInProgress)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	rec, created, err := store.Idempotency().Reserve(context.Background(), "k", "h")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !created || rec.Key != "k" || rec.RequestHash != "h" || rec.State != app.IdempotencyInProgress {
		t.Fatalf("unexpected record: created=%v rec=%+v", created, rec)
	}
	assertExpectations(t, mock)
}

func TestIdempotencyRepo_ReserveReturnsExistingCompleted(t *testing.T) {
	store, mock := newTestStore(t)
	// Insert-first: a duplicate-key (1062) on INSERT means the key is already
	// owned, so Reserve falls through to read the committed record.
	mock.ExpectExec(`INSERT INTO idempotency_keys (idempotency_key, request_hash, state)
		  VALUES (?, ?, ?)`).
		WithArgs("k", "h", string(app.IdempotencyInProgress)).
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'k' for key 'PRIMARY'"})
	mock.ExpectQuery(selectIdempotencyByKey).
		WithArgs("k").
		WillReturnRows(sqlmock.NewRows([]string{
			"idempotency_key", "request_hash", "state", "response_status", "response_body",
		}).AddRow("k", "h", string(app.IdempotencyCompleted), 200, `{"ok":true}`))

	rec, created, err := store.Idempotency().Reserve(context.Background(), "k", "h")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if created || rec.State != app.IdempotencyCompleted || rec.ResponseStatus != 200 || string(rec.ResponseBody) != `{"ok":true}` {
		t.Fatalf("unexpected record: created=%v rec=%+v", created, rec)
	}
	assertExpectations(t, mock)
}

func TestIdempotencyRepo_Complete(t *testing.T) {
	store, mock := newTestStore(t)
	mock.ExpectExec(`UPDATE idempotency_keys
		    SET state = ?, response_status = ?, response_body = ?
		  WHERE idempotency_key = ?`).
		WithArgs(string(app.IdempotencyCompleted), 200, `{"ok":true}`, "k").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Idempotency().Complete(context.Background(), "k", 200, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	assertExpectations(t, mock)
}

func TestIdempotencyRepo_Abort(t *testing.T) {
	store, mock := newTestStore(t)
	mock.ExpectExec(`DELETE FROM idempotency_keys WHERE idempotency_key = ?`).
		WithArgs("k").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Idempotency().Abort(context.Background(), "k"); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	assertExpectations(t, mock)
}
