package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/iberkayC/case1back/internal/domain"
)

type MatchRepo struct {
	db *Store
}

func (r *MatchRepo) InsertFixtures(ctx context.Context, matches []domain.Match) ([]domain.Match, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	q := r.db.withTxOrDB(ctx)

	values := make([]string, 0, len(matches))
	args := make([]any, 0, len(matches)*4)
	for _, m := range matches {
		values = append(values, "(?, ?, ?, ?)")
		args = append(args, m.LeagueID, m.Week, m.HomeTeamID, m.AwayTeamID)
	}
	stmt := "INSERT INTO matches (league_id, week, home_team_id, away_team_id) VALUES " +
		strings.Join(values, ", ")

	if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
		return nil, fmt.Errorf("insert fixtures: %w", err)
	}

	// Match by (week, home_team_id) to recover the new IDs rather than assuming
	// firstID+i, to be safe. Only runs at league creation, so every match here
	// is one we just inserted.
	idsByKey, err := fixtureIDsByKey(ctx, q, matches[0].LeagueID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Match, len(matches))
	for i, m := range matches {
		m.ID = idsByKey[fixtureKey{week: m.Week, homeTeamID: m.HomeTeamID}]
		out[i] = m
	}
	return out, nil
}

type fixtureKey struct {
	week       int
	homeTeamID int64
}

// fixtureIDsByKey maps (week, home_team_id) to id for every match in the league,
// recovering generated IDs after a bulk insert without assuming contiguous
// auto-increment values.
func fixtureIDsByKey(ctx context.Context, q dbtx, leagueID int64) (map[fixtureKey]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, week, home_team_id FROM matches WHERE league_id = ?`, leagueID)
	if err != nil {
		return nil, fmt.Errorf("read back fixture ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[fixtureKey]int64)
	for rows.Next() {
		var (
			id   int64
			week int
			home int64
		)
		if err := rows.Scan(&id, &week, &home); err != nil {
			return nil, fmt.Errorf("scan fixture id: %w", err)
		}
		out[fixtureKey{week: week, homeTeamID: home}] = id
	}
	return out, rows.Err()
}

// RecordResult writes goals; sets edited_at when edited is true, else stamps
// played_at if not already set.
func (r *MatchRepo) RecordResult(ctx context.Context, matchID int64, homeGoals, awayGoals int, edited bool) error {
	q := r.db.withTxOrDB(ctx)
	if edited {
		_, err := q.ExecContext(ctx,
			`UPDATE matches
			    SET home_goals = ?, away_goals = ?, edited_at = CURRENT_TIMESTAMP(3)
			  WHERE id = ?`,
			homeGoals, awayGoals, matchID,
		)
		if err != nil {
			return fmt.Errorf("edit match result: %w", err)
		}
		return nil
	}
	_, err := q.ExecContext(ctx,
		`UPDATE matches
		    SET home_goals = ?, away_goals = ?,
		        played_at = COALESCE(played_at, CURRENT_TIMESTAMP(3))
		  WHERE id = ?`,
		homeGoals, awayGoals, matchID,
	)
	if err != nil {
		return fmt.Errorf("record match result: %w", err)
	}
	return nil
}

func (r *MatchRepo) ClearAllResults(ctx context.Context, leagueID int64) error {
	_, err := r.db.withTxOrDB(ctx).ExecContext(ctx,
		`UPDATE matches
		    SET home_goals = NULL, away_goals = NULL,
		        played_at = NULL, edited_at = NULL
		  WHERE league_id = ?`, leagueID,
	)
	if err != nil {
		return fmt.Errorf("clear results: %w", err)
	}
	return nil
}

func (r *MatchRepo) Get(ctx context.Context, matchID int64) (domain.Match, error) {
	row := r.db.withTxOrDB(ctx).QueryRowContext(ctx,
		`SELECT id, league_id, week, home_team_id, away_team_id,
		        home_goals, away_goals, played_at, edited_at
		   FROM matches WHERE id = ?`, matchID,
	)
	return scanMatch(row)
}

func (r *MatchRepo) List(ctx context.Context, leagueID int64) ([]domain.Match, error) {
	rows, err := r.db.withTxOrDB(ctx).QueryContext(ctx,
		`SELECT id, league_id, week, home_team_id, away_team_id,
		        home_goals, away_goals, played_at, edited_at
		   FROM matches WHERE league_id = ? ORDER BY week ASC, id ASC`, leagueID,
	)
	if err != nil {
		return nil, fmt.Errorf("list matches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanMatches(rows)
}

func (r *MatchRepo) ListByWeek(ctx context.Context, leagueID int64, week int) ([]domain.Match, error) {
	rows, err := r.db.withTxOrDB(ctx).QueryContext(ctx,
		`SELECT id, league_id, week, home_team_id, away_team_id,
		        home_goals, away_goals, played_at, edited_at
		   FROM matches WHERE league_id = ? AND week = ? ORDER BY id ASC`, leagueID, week,
	)
	if err != nil {
		return nil, fmt.Errorf("list matches by week: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanMatches(rows)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMatch(row rowScanner) (domain.Match, error) {
	var (
		m        domain.Match
		hGoals   sql.NullInt64
		aGoals   sql.NullInt64
		playedAt sql.NullTime
		editedAt sql.NullTime
	)
	err := row.Scan(&m.ID, &m.LeagueID, &m.Week, &m.HomeTeamID, &m.AwayTeamID,
		&hGoals, &aGoals, &playedAt, &editedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Match{}, domain.ErrMatchNotFound
	}
	if err != nil {
		return domain.Match{}, fmt.Errorf("scan match: %w", err)
	}
	if hGoals.Valid {
		v := int(hGoals.Int64)
		m.HomeGoals = &v
	}
	if aGoals.Valid {
		v := int(aGoals.Int64)
		m.AwayGoals = &v
	}
	if playedAt.Valid {
		t := playedAt.Time
		m.PlayedAt = &t
	}
	if editedAt.Valid {
		t := editedAt.Time
		m.EditedAt = &t
	}
	return m, nil
}

func scanMatches(rows *sql.Rows) ([]domain.Match, error) {
	var out []domain.Match
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate matches: %w", err)
	}
	return out, nil
}
