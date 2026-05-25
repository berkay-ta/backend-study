package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type txKey struct{}

// WithTx runs fn inside a MySQL transaction. Errors or panics trigger
// rollback; otherwise commit. Implements app.TxRunner.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return fn(ctx)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = fmt.Errorf("%w; rollback: %v", err, rbErr)
			}
			return
		}
		if cErr := tx.Commit(); cErr != nil {
			err = fmt.Errorf("commit tx: %w", cErr)
		}
	}()

	err = fn(context.WithValue(ctx, txKey{}, tx))
	return err
}

type dbtx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func (s *Store) withTxOrDB(ctx context.Context) dbtx {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return s.db
}
