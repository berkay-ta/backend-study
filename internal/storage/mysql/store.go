// Package mysql implements the application's repository interfaces.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	db *sql.DB
}

type Options struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Open(ctx context.Context, dsn string, opts Options) (*Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open mysql: %w", err)
	}
	if opts.MaxOpenConns > 0 {
		db.SetMaxOpenConns(opts.MaxOpenConns)
	}
	if opts.MaxIdleConns > 0 {
		db.SetMaxIdleConns(opts.MaxIdleConns)
	}
	if opts.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(opts.ConnMaxLifetime)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("mysql ping: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Leagues() *LeagueRepo          { return &LeagueRepo{db: s} }
func (s *Store) Matches() *MatchRepo           { return &MatchRepo{db: s} }
func (s *Store) Snapshots() *SnapshotRepo      { return &SnapshotRepo{db: s} }
func (s *Store) Predictions() *PredictionRepo  { return &PredictionRepo{db: s} }
func (s *Store) Idempotency() *IdempotencyRepo { return &IdempotencyRepo{db: s} }
