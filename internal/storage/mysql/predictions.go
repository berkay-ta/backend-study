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

type PredictionRepo struct {
	db *Store
}

func (r *PredictionRepo) Create(ctx context.Context, run domain.PredictionRun) (domain.PredictionRun, error) {
	q := r.db.withTxOrDB(ctx)
	res, err := q.ExecContext(ctx,
		`INSERT INTO prediction_runs (league_id, strategy, snapshot_id, league_version, params_json, notes, stale)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.LeagueID, string(run.Strategy), run.SnapshotID, run.LeagueVersion,
		nullableJSON(run.ParamsJSON), nullableString(run.Result.Notes), run.Stale,
	)
	if err != nil {
		return domain.PredictionRun{}, fmt.Errorf("insert prediction_run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.PredictionRun{}, fmt.Errorf("prediction_run last insert id: %w", err)
	}
	run.ID = id

	if len(run.Result.Entries) > 0 {
		values := make([]string, 0, len(run.Result.Entries))
		args := make([]any, 0, len(run.Result.Entries)*13)
		for _, e := range run.Result.Entries {
			values = append(values, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args,
				id, e.TeamID, e.ProjectedPosition, e.ChampionshipPct, e.AveragePosition, e.Played,
				e.ExpectedWon, e.ExpectedDrawn, e.ExpectedLost,
				e.ExpectedGoalsFor, e.ExpectedGoalsAgainst, e.ExpectedGoalDiff, e.ExpectedPoints,
			)
		}
		stmt := `INSERT INTO prediction_percentages
		         (run_id, team_id, projected_position, championship_pct, average_position, played,
		          expected_won, expected_drawn, expected_lost, expected_goals_for, expected_goals_against,
		          expected_goal_diff, expected_points) VALUES ` +
			strings.Join(values, ", ")
		if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
			return domain.PredictionRun{}, fmt.Errorf("insert prediction_percentages: %w", err)
		}
	}
	return run, nil
}

func (r *PredictionRepo) Get(ctx context.Context, runID int64) (domain.PredictionRun, error) {
	q := r.db.withTxOrDB(ctx)
	row, err := scanPredictionRun(ctx, q,
		`SELECT id, league_id, strategy, snapshot_id, league_version, created_at, COALESCE(notes,''), stale
		   FROM prediction_runs WHERE id = ?`, runID,
	)
	if err != nil {
		return domain.PredictionRun{}, err
	}
	entries, err := loadEntriesForRuns(ctx, q, []int64{row.ID})
	if err != nil {
		return domain.PredictionRun{}, err
	}
	row.Result.Entries = entries[row.ID]
	return row, nil
}

func (r *PredictionRepo) ListByLeague(ctx context.Context, leagueID int64) ([]domain.PredictionRun, error) {
	q := r.db.withTxOrDB(ctx)
	rows, err := q.QueryContext(ctx,
		`SELECT id, league_id, strategy, snapshot_id, league_version, created_at, COALESCE(notes,''), stale
		   FROM prediction_runs WHERE league_id = ? ORDER BY id DESC`, leagueID,
	)
	if err != nil {
		return nil, fmt.Errorf("list predictions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []domain.PredictionRun
	for rows.Next() {
		var (
			run     domain.PredictionRun
			created time.Time
			strat   string
		)
		if err := rows.Scan(&run.ID, &run.LeagueID, &strat, &run.SnapshotID, &run.LeagueVersion,
			&created, &run.Result.Notes, &run.Stale); err != nil {
			return nil, fmt.Errorf("scan prediction_run: %w", err)
		}
		run.Strategy = domain.Strategy(strat)
		run.Result.Strategy = run.Strategy
		run.CreatedAt = created
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate predictions: %w", err)
	}
	if len(runs) == 0 {
		return runs, nil
	}

	entries, err := loadEntriesForRuns(ctx, q, runIDsOf(runs))
	if err != nil {
		return nil, err
	}
	for i := range runs {
		runs[i].Result.Entries = entries[runs[i].ID]
	}
	return runs, nil
}

func runIDsOf(runs []domain.PredictionRun) []int64 {
	ids := make([]int64, len(runs))
	for i, r := range runs {
		ids[i] = r.ID
	}
	return ids
}

func (r *PredictionRepo) MarkStaleBefore(ctx context.Context, leagueID int64, newVersion int64) error {
	_, err := r.db.withTxOrDB(ctx).ExecContext(ctx,
		`UPDATE prediction_runs SET stale = TRUE
		  WHERE league_id = ? AND league_version < ? AND stale = FALSE`,
		leagueID, newVersion,
	)
	if err != nil {
		return fmt.Errorf("mark predictions stale: %w", err)
	}
	return nil
}

// loadEntriesForRuns fetches predicted final-table rows for the given run IDs
// in one query, bucketed by run_id with per-run table ordering.
func loadEntriesForRuns(ctx context.Context, q dbtx, runIDs []int64) (map[int64][]domain.PredictionEntry, error) {
	placeholders := make([]string, len(runIDs))
	args := make([]any, len(runIDs))
	for i, id := range runIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	stmt := `SELECT pp.run_id, pp.team_id, t.name, pp.projected_position, pp.championship_pct,
	                  pp.average_position, pp.played, pp.expected_won, pp.expected_drawn,
	                  pp.expected_lost, pp.expected_goals_for, pp.expected_goals_against,
	                  pp.expected_goal_diff, pp.expected_points
	           FROM prediction_percentages pp
	           JOIN teams t ON t.id = pp.team_id
	          WHERE pp.run_id IN (` + strings.Join(placeholders, ", ") + `)
	          ORDER BY pp.run_id ASC, pp.projected_position ASC`
	rows, err := q.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("load prediction entries (batch): %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int64][]domain.PredictionEntry, len(runIDs))
	for rows.Next() {
		var (
			runID int64
			e     domain.PredictionEntry
		)
		if err := rows.Scan(
			&runID, &e.TeamID, &e.TeamName, &e.ProjectedPosition, &e.ChampionshipPct,
			&e.AveragePosition, &e.Played, &e.ExpectedWon, &e.ExpectedDrawn, &e.ExpectedLost,
			&e.ExpectedGoalsFor, &e.ExpectedGoalsAgainst, &e.ExpectedGoalDiff, &e.ExpectedPoints,
		); err != nil {
			return nil, fmt.Errorf("scan prediction entry: %w", err)
		}
		out[runID] = append(out[runID], e)
	}
	return out, rows.Err()
}

func scanPredictionRun(ctx context.Context, q dbtx, query string, args ...any) (domain.PredictionRun, error) {
	var (
		run     domain.PredictionRun
		created time.Time
		strat   string
	)
	err := q.QueryRowContext(ctx, query, args...).Scan(
		&run.ID, &run.LeagueID, &strat, &run.SnapshotID, &run.LeagueVersion,
		&created, &run.Result.Notes, &run.Stale,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PredictionRun{}, domain.ErrPredictionNotFound
	}
	if err != nil {
		return domain.PredictionRun{}, fmt.Errorf("get prediction_run: %w", err)
	}
	run.Strategy = domain.Strategy(strat)
	run.Result.Strategy = run.Strategy
	run.CreatedAt = created
	return run, nil
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
