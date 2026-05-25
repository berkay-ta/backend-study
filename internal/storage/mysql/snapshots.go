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

type SnapshotRepo struct {
	db *Store
}

func (r *SnapshotRepo) GetOrCreate(ctx context.Context, snap domain.StandingSnapshot) (domain.StandingSnapshot, error) {
	q := r.db.withTxOrDB(ctx)
	res, err := q.ExecContext(ctx,
		`INSERT INTO standing_snapshots (league_id, snapshot_week, league_version)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
		snap.LeagueID, snap.SnapshotWeek, snap.LeagueVersion,
	)
	if err != nil {
		return domain.StandingSnapshot{}, fmt.Errorf("get or create snapshot: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.StandingSnapshot{}, fmt.Errorf("snapshot last insert id: %w", err)
	}
	snap.ID = id

	if len(snap.Rows) > 0 {
		values := make([]string, 0, len(snap.Rows))
		args := make([]any, 0, len(snap.Rows)*11)
		for _, row := range snap.Rows {
			values = append(values, "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
			args = append(args,
				id, row.TeamID, row.Position, row.Played, row.Won, row.Drawn, row.Lost,
				row.GoalsFor, row.GoalsAgainst, row.GoalDiff, row.Points,
			)
		}
		stmt := `INSERT IGNORE INTO standing_rows
		         (snapshot_id, team_id, position, played, won, drawn, lost,
		          goals_for, goals_against, goal_diff, points) VALUES ` +
			strings.Join(values, ", ")
		if _, err := q.ExecContext(ctx, stmt, args...); err != nil {
			return domain.StandingSnapshot{}, fmt.Errorf("insert standing_rows: %w", err)
		}
	}
	return snap, nil
}

func (r *SnapshotRepo) Get(ctx context.Context, snapshotID int64) (domain.StandingSnapshot, error) {
	q := r.db.withTxOrDB(ctx)
	var (
		snap    domain.StandingSnapshot
		created time.Time
	)
	err := q.QueryRowContext(ctx,
		`SELECT id, league_id, snapshot_week, league_version, created_at
		   FROM standing_snapshots WHERE id = ?`, snapshotID,
	).Scan(&snap.ID, &snap.LeagueID, &snap.SnapshotWeek, &snap.LeagueVersion, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.StandingSnapshot{}, domain.ErrSnapshotNotFound
	}
	if err != nil {
		return domain.StandingSnapshot{}, fmt.Errorf("get snapshot: %w", err)
	}
	snap.CreatedAt = created

	rows, err := q.QueryContext(ctx,
		`SELECT sr.team_id, t.name, sr.position, sr.played, sr.won, sr.drawn, sr.lost,
		        sr.goals_for, sr.goals_against, sr.goal_diff, sr.points
		   FROM standing_rows sr
		   JOIN teams t ON t.id = sr.team_id
		  WHERE sr.snapshot_id = ?
		  ORDER BY sr.position ASC`, snapshotID,
	)
	if err != nil {
		return domain.StandingSnapshot{}, fmt.Errorf("get standing_rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row domain.StandingRow
		if err := rows.Scan(&row.TeamID, &row.TeamName, &row.Position, &row.Played, &row.Won, &row.Drawn, &row.Lost,
			&row.GoalsFor, &row.GoalsAgainst, &row.GoalDiff, &row.Points); err != nil {
			return domain.StandingSnapshot{}, fmt.Errorf("scan standing row: %w", err)
		}
		snap.Rows = append(snap.Rows, row)
	}
	return snap, rows.Err()
}
