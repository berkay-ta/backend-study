package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iberkayC/case1back/internal/domain"
)

func TestPredictionRepo_Create(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`INSERT INTO prediction_runs (league_id, strategy, snapshot_id, league_version, params_json, notes, stale)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`).
		WithArgs(int64(1), "monte_carlo", int64(77), int64(2), nil, nil, false).
		WillReturnResult(sqlmock.NewResult(33, 1))

	mock.ExpectExec("INSERT INTO prediction_percentages\n\t\t         (run_id, team_id, projected_position, championship_pct, average_position, played,\n\t\t          expected_won, expected_drawn, expected_lost, expected_goals_for, expected_goals_against,\n\t\t          expected_goal_diff, expected_points) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)").
		WithArgs(
			int64(33), int64(10), 1, 60.0, 1.4, 6, 4.0, 2.0, 0.0, 14.0, 6.0, 8.0, 14.0,
			int64(33), int64(11), 2, 40.0, 2.1, 6, 3.0, 3.0, 0.0, 12.0, 7.0, 5.0, 12.0,
		).
		WillReturnResult(sqlmock.NewResult(0, 2))

	run := domain.PredictionRun{
		LeagueID:      1,
		Strategy:      domain.Strategy("monte_carlo"),
		SnapshotID:    77,
		LeagueVersion: 2,
		Result: domain.PredictionResult{
			Entries: []domain.PredictionEntry{
				{ProjectedPosition: 1, TeamID: 10, ChampionshipPct: 60, AveragePosition: 1.4, Played: 6,
					ExpectedWon: 4, ExpectedDrawn: 2, ExpectedLost: 0, ExpectedGoalsFor: 14,
					ExpectedGoalsAgainst: 6, ExpectedGoalDiff: 8, ExpectedPoints: 14},
				{ProjectedPosition: 2, TeamID: 11, ChampionshipPct: 40, AveragePosition: 2.1, Played: 6,
					ExpectedWon: 3, ExpectedDrawn: 3, ExpectedLost: 0, ExpectedGoalsFor: 12,
					ExpectedGoalsAgainst: 7, ExpectedGoalDiff: 5, ExpectedPoints: 12},
			},
		},
	}
	got, err := store.Predictions().Create(context.Background(), run)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != 33 {
		t.Errorf("id = %d, want 33", got.ID)
	}
	assertExpectations(t, mock)
}

func TestPredictionRepo_Get_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, league_id, strategy, snapshot_id, league_version, created_at, COALESCE(notes,''), stale
			   FROM prediction_runs WHERE id = ?`).
		WithArgs(int64(99)).
		WillReturnError(sqlNoRows())

	_, err := store.Predictions().Get(context.Background(), 99)
	if !errors.Is(err, domain.ErrPredictionNotFound) {
		t.Errorf("expected ErrPredictionNotFound, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestPredictionRepo_Get(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, league_id, strategy, snapshot_id, league_version, created_at, COALESCE(notes,''), stale
			   FROM prediction_runs WHERE id = ?`).
		WithArgs(int64(33)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "league_id", "strategy", "snapshot_id", "league_version", "created_at", "notes", "stale",
		}).AddRow(int64(33), int64(1), "monte_carlo", int64(77), int64(2), now, "", false))

	mock.ExpectQuery("SELECT pp.run_id, pp.team_id, t.name, pp.projected_position, pp.championship_pct,\n\t\t                  pp.average_position, pp.played, pp.expected_won, pp.expected_drawn,\n\t\t                  pp.expected_lost, pp.expected_goals_for, pp.expected_goals_against,\n\t\t                  pp.expected_goal_diff, pp.expected_points\n\t\t           FROM prediction_percentages pp\n\t\t           JOIN teams t ON t.id = pp.team_id\n\t\t          WHERE pp.run_id IN (?)\n\t\t          ORDER BY pp.run_id ASC, pp.projected_position ASC").
		WithArgs(int64(33)).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "team_id", "name", "projected_position", "championship_pct",
			"average_position", "played", "expected_won", "expected_drawn", "expected_lost",
			"expected_goals_for", "expected_goals_against", "expected_goal_diff", "expected_points",
		}).
			AddRow(int64(33), int64(10), "Alpha", 1, 60.0, 1.4, 6, 4.0, 2.0, 0.0, 14.0, 6.0, 8.0, 14.0).
			AddRow(int64(33), int64(11), "Bravo", 2, 40.0, 2.1, 6, 3.0, 3.0, 0.0, 12.0, 7.0, 5.0, 12.0))

	got, err := store.Predictions().Get(context.Background(), 33)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != 33 || len(got.Result.Entries) != 2 || got.Result.Entries[0].TeamName != "Alpha" {
		t.Errorf("got = %+v", got)
	}
	assertExpectations(t, mock)
}

func TestPredictionRepo_MarkStaleBefore(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`UPDATE prediction_runs SET stale = TRUE
			  WHERE league_id = ? AND league_version < ? AND stale = FALSE`).
		WithArgs(int64(1), int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 3))

	if err := store.Predictions().MarkStaleBefore(context.Background(), 1, 5); err != nil {
		t.Fatalf("MarkStaleBefore: %v", err)
	}
	assertExpectations(t, mock)
}
