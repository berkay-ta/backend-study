package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/iberkayC/case1back/internal/app"
)

type IdempotencyRepo struct{ db *Store }

func (r *IdempotencyRepo) Reserve(ctx context.Context, key string, requestHash string) (app.IdempotencyRecord, bool, error) {
	q := r.db.withTxOrDB(ctx)

	// Insert-first: the PK picks the winner. Losers get a 1062 and read the
	// committed record below. No SELECT ... FOR UPDATE first; under
	// REPEATABLE READ that gap-locks and concurrent inserts deadlock (1213).
	// If we SELECT ... FOR UPDATE first, on a non-existent row, we get a gap-lock
	// two txns can both hold the gap lock, then if they both try to insert, they both
	// need a lock for the insertion, but they both had a gaplock, so deadlock.
	// If we SELECT first without locks, need retries.
	// In this case though, the InProgress state is never committed, and the
	// state can be inferred from response_body IS NULL, but chose to keep.
	_, err := q.ExecContext(ctx,
		`INSERT INTO idempotency_keys (idempotency_key, request_hash, state)
		  VALUES (?, ?, ?)`,
		key, requestHash, string(app.IdempotencyInProgress),
	)
	if err == nil {
		return app.IdempotencyRecord{
			Key:         key,
			RequestHash: requestHash,
			State:       app.IdempotencyInProgress,
		}, true, nil
	}
	if !isDuplicateKey(err) {
		return app.IdempotencyRecord{}, false, fmt.Errorf("reserve idempotency key: %w", err)
	}

	// The INSERT blocked on the owner's uncommitted row, so a 1062 means it
	// committed and is now visible. Plain read, not FOR UPDATE: the record is
	// immutable, and upgrading our shared lock would deadlock (1213).
	rec, err := r.getByKey(ctx, q, key)
	if err != nil {
		return app.IdempotencyRecord{}, false, fmt.Errorf("get idempotency key after duplicate: %w", err)
	}
	return rec, false, nil
}

func (r *IdempotencyRepo) Complete(ctx context.Context, key string, status int, body []byte) error {
	res, err := r.db.withTxOrDB(ctx).ExecContext(ctx,
		`UPDATE idempotency_keys
		    SET state = ?, response_status = ?, response_body = ?
		  WHERE idempotency_key = ?`,
		string(app.IdempotencyCompleted), status, string(body), key,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *IdempotencyRepo) Abort(ctx context.Context, key string) error {
	if _, err := r.db.withTxOrDB(ctx).ExecContext(ctx,
		`DELETE FROM idempotency_keys WHERE idempotency_key = ?`, key,
	); err != nil {
		return fmt.Errorf("abort idempotency key: %w", err)
	}
	return nil
}

func (r *IdempotencyRepo) getByKey(ctx context.Context, q dbtx, key string) (app.IdempotencyRecord, error) {
	var (
		rec    app.IdempotencyRecord
		state  string
		status sql.NullInt64
		body   sql.NullString
	)
	err := q.QueryRowContext(ctx,
		`SELECT idempotency_key, request_hash, state, response_status, response_body
		   FROM idempotency_keys
		  WHERE idempotency_key = ?`,
		key,
	).Scan(&rec.Key, &rec.RequestHash, &state, &status, &body)
	if err != nil {
		return app.IdempotencyRecord{}, err
	}
	rec.State = app.IdempotencyState(state)
	if status.Valid {
		rec.ResponseStatus = int(status.Int64)
	}
	if body.Valid {
		rec.ResponseBody = []byte(body.String)
	}
	return rec, nil
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
