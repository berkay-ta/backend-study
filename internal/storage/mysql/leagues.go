package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iberkayC/case1back/internal/domain"
)

type LeagueRepo struct {
	db *Store
}

func (r *LeagueRepo) Create(ctx context.Context, name string, totalWeeks int, teams []domain.Team) (domain.League, []domain.Team, error) {
	q := r.db.withTxOrDB(ctx)
	res, err := q.ExecContext(ctx,
		`INSERT INTO leagues (name, current_week, total_weeks, version) VALUES (?, 1, ?, 0)`,
		name, totalWeeks,
	)
	if err != nil {
		return domain.League{}, nil, fmt.Errorf("insert league: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.League{}, nil, fmt.Errorf("last insert id (league): %w", err)
	}

	out := make([]domain.Team, len(teams))
	if len(teams) > 0 {
		values := make([]string, len(teams))
		args := make([]any, 0, len(teams)*3)
		for i, t := range teams {
			values[i] = "(?, ?, ?)"
			args = append(args, id, t.Name, t.Strength)
		}
		stmt := "INSERT INTO teams (league_id, name, strength) VALUES " + strings.Join(values, ", ")
		if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
			return domain.League{}, nil, fmt.Errorf("insert teams: %w", err)
		}
		// Match by name to recover the new IDs rather than assuming firstID+i,
		// to be safe. Names are unique per league.
		idsByName, err := teamIDsByName(ctx, q, id)
		if err != nil {
			return domain.League{}, nil, err
		}
		for i, t := range teams {
			out[i] = domain.Team{
				ID:       idsByName[t.Name],
				LeagueID: id,
				Name:     t.Name,
				Strength: t.Strength,
			}
		}
	}

	league, err := r.getByID(ctx, q, id)
	if err != nil {
		return domain.League{}, nil, err
	}
	return league, out, nil
}

func (r *LeagueRepo) List(ctx context.Context) ([]domain.League, error) {
	q := r.db.withTxOrDB(ctx)
	rows, err := q.QueryContext(ctx,
		`SELECT id, name, current_week, total_weeks, version, created_at FROM leagues ORDER BY id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list leagues: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.League
	for rows.Next() {
		var l domain.League
		var created time.Time
		if err := rows.Scan(&l.ID, &l.Name, &l.CurrentWeek, &l.TotalWeeks, &l.Version, &created); err != nil {
			return nil, fmt.Errorf("scan league: %w", err)
		}
		l.CreatedAt = created
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r *LeagueRepo) Get(ctx context.Context, id int64) (domain.League, error) {
	return r.getByID(ctx, r.db.withTxOrDB(ctx), id)
}

// GetForUpdate locks the league row for the rest of the transaction; must be
// called inside WithTx.
func (r *LeagueRepo) GetForUpdate(ctx context.Context, id int64) (domain.League, error) {
	q := r.db.withTxOrDB(ctx)
	var (
		l       domain.League
		created time.Time
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, name, current_week, total_weeks, version, created_at
		   FROM leagues
		  WHERE id = ?
		  FOR UPDATE`, id,
	).Scan(&l.ID, &l.Name, &l.CurrentWeek, &l.TotalWeeks, &l.Version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.League{}, domain.ErrLeagueNotFound
	}
	if err != nil {
		return domain.League{}, fmt.Errorf("get league for update: %w", err)
	}
	l.CreatedAt = created
	return l, nil
}

func (r *LeagueRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.withTxOrDB(ctx).ExecContext(ctx, `DELETE FROM leagues WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete league: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrLeagueNotFound
	}
	return nil
}

// BumpVersion increments the league's mutation counter and returns the new
// value. It's a staleness cursor for predictions, not optimistic locking,
// concurrency safety comes from the GetForUpdate every mutation takes first.
func (r *LeagueRepo) BumpVersion(ctx context.Context, id int64) (int64, error) {
	q := r.db.withTxOrDB(ctx)
	if _, err := q.ExecContext(ctx, `UPDATE leagues SET version = version + 1 WHERE id = ?`, id); err != nil {
		return 0, fmt.Errorf("bump version: %w", err)
	}
	var v int64
	if err := q.QueryRowContext(ctx, `SELECT version FROM leagues WHERE id = ?`, id).Scan(&v); err != nil {
		return 0, fmt.Errorf("read version: %w", err)
	}
	return v, nil
}

func (r *LeagueRepo) SetCurrentWeek(ctx context.Context, id int64, week int) error {
	_, err := r.db.withTxOrDB(ctx).ExecContext(ctx,
		`UPDATE leagues SET current_week = ? WHERE id = ?`, week, id,
	)
	if err != nil {
		return fmt.Errorf("set current_week: %w", err)
	}
	return nil
}

func (r *LeagueRepo) GetTeams(ctx context.Context, leagueID int64) ([]domain.Team, error) {
	rows, err := r.db.withTxOrDB(ctx).QueryContext(ctx,
		`SELECT id, league_id, name, strength FROM teams WHERE league_id = ? ORDER BY name ASC`,
		leagueID,
	)
	if err != nil {
		return nil, fmt.Errorf("get teams: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Team
	for rows.Next() {
		var t domain.Team
		if err := rows.Scan(&t.ID, &t.LeagueID, &t.Name, &t.Strength); err != nil {
			return nil, fmt.Errorf("scan team: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// teamIDsByName returns a name-to-id map for every team in the league. Used to
// recover generated IDs after a bulk insert without assuming contiguous
// auto-increment values.
func teamIDsByName(ctx context.Context, q dbtx, leagueID int64) (map[string]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, name FROM teams WHERE league_id = ?`, leagueID)
	if err != nil {
		return nil, fmt.Errorf("read back team ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]int64)
	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan team id: %w", err)
		}
		out[name] = id
	}
	return out, rows.Err()
}

func (r *LeagueRepo) getByID(ctx context.Context, q dbtx, id int64) (domain.League, error) {
	var (
		l       domain.League
		created time.Time
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, name, current_week, total_weeks, version, created_at
		   FROM leagues WHERE id = ?`, id,
	).Scan(&l.ID, &l.Name, &l.CurrentWeek, &l.TotalWeeks, &l.Version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.League{}, domain.ErrLeagueNotFound
	}
	if err != nil {
		return domain.League{}, fmt.Errorf("get league: %w", err)
	}
	l.CreatedAt = created
	return l, nil
}
