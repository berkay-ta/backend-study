package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iberkayC/case1back/internal/domain"
)

func TestSnapshotRepo_GetOrCreate(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`INSERT INTO standing_snapshots (league_id, snapshot_week, league_version)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`).
		WithArgs(int64(1), 3, int64(2)).
		WillReturnResult(sqlmock.NewResult(77, 1))

	mock.ExpectExec("INSERT IGNORE INTO standing_rows\n\t\t         (snapshot_id, team_id, position, played, won, drawn, lost,\n\t\t          goals_for, goals_against, goal_diff, points) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)").
		WithArgs(int64(77), int64(10), 1, 3, 2, 1, 0, 5, 2, 3, 7).
		WillReturnResult(sqlmock.NewResult(0, 1))

	snap := domain.StandingSnapshot{
		LeagueID:      1,
		SnapshotWeek:  3,
		LeagueVersion: 2,
		Rows: []domain.StandingRow{
			{TeamID: 10, Position: 1, Played: 3, Won: 2, Drawn: 1, Lost: 0,
				GoalsFor: 5, GoalsAgainst: 2, GoalDiff: 3, Points: 7},
		},
	}
	got, err := store.Snapshots().GetOrCreate(context.Background(), snap)
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if got.ID != 77 {
		t.Errorf("id = %d, want 77", got.ID)
	}
	assertExpectations(t, mock)
}

func TestSnapshotRepo_Get_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, league_id, snapshot_week, league_version, created_at
			   FROM standing_snapshots WHERE id = ?`).
		WithArgs(int64(500)).
		WillReturnError(sqlNoRows())

	_, err := store.Snapshots().Get(context.Background(), 500)
	if !errors.Is(err, domain.ErrSnapshotNotFound) {
		t.Errorf("expected ErrSnapshotNotFound, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestSnapshotRepo_Get(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, league_id, snapshot_week, league_version, created_at
			   FROM standing_snapshots WHERE id = ?`).
		WithArgs(int64(77)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "league_id", "snapshot_week", "league_version", "created_at"}).
			AddRow(int64(77), int64(1), 3, int64(2), now))

	mock.ExpectQuery(`SELECT sr.team_id, t.name, sr.position, sr.played, sr.won, sr.drawn, sr.lost,
			        sr.goals_for, sr.goals_against, sr.goal_diff, sr.points
			   FROM standing_rows sr
			   JOIN teams t ON t.id = sr.team_id
			  WHERE sr.snapshot_id = ?
			  ORDER BY sr.position ASC`).
		WithArgs(int64(77)).
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id", "name", "position", "played", "won", "drawn", "lost",
			"goals_for", "goals_against", "goal_diff", "points",
		}).AddRow(int64(10), "Alpha", 1, 3, 2, 1, 0, 5, 2, 3, 7))

	snap, err := store.Snapshots().Get(context.Background(), 77)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if snap.ID != 77 || len(snap.Rows) != 1 || snap.Rows[0].TeamName != "Alpha" {
		t.Errorf("snap = %+v", snap)
	}
	assertExpectations(t, mock)
}
