package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iberkayC/case1back/internal/domain"
)

func TestMatchRepo_InsertFixtures_Empty(t *testing.T) {
	store, mock := newTestStore(t)

	out, err := store.Matches().InsertFixtures(context.Background(), nil)
	if err != nil {
		t.Fatalf("InsertFixtures(nil): %v", err)
	}
	if out != nil {
		t.Errorf("expected nil, got %+v", out)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_InsertFixtures(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec("INSERT INTO matches (league_id, week, home_team_id, away_team_id) VALUES (?, ?, ?, ?), (?, ?, ?, ?)").
		WithArgs(int64(1), 1, int64(10), int64(11), int64(1), 1, int64(12), int64(13)).
		WillReturnResult(sqlmock.NewResult(100, 2))

	mock.ExpectQuery(`SELECT id, week, home_team_id FROM matches WHERE league_id = ?`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "week", "home_team_id"}).
			AddRow(int64(100), 1, int64(10)).
			AddRow(int64(101), 1, int64(12)))

	in := []domain.Match{
		{LeagueID: 1, Week: 1, HomeTeamID: 10, AwayTeamID: 11},
		{LeagueID: 1, Week: 1, HomeTeamID: 12, AwayTeamID: 13},
	}
	out, err := store.Matches().InsertFixtures(context.Background(), in)
	if err != nil {
		t.Fatalf("InsertFixtures: %v", err)
	}
	if len(out) != 2 || out[0].ID != 100 || out[1].ID != 101 {
		t.Errorf("out = %+v", out)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_RecordResult_Played(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`UPDATE matches
			    SET home_goals = ?, away_goals = ?,
			        played_at = COALESCE(played_at, CURRENT_TIMESTAMP(3))
			  WHERE id = ?`).
		WithArgs(2, 1, int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Matches().RecordResult(context.Background(), 50, 2, 1, false); err != nil {
		t.Fatalf("RecordResult: %v", err)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_RecordResult_Edited(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`UPDATE matches
			    SET home_goals = ?, away_goals = ?, edited_at = CURRENT_TIMESTAMP(3)
			  WHERE id = ?`).
		WithArgs(3, 3, int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.Matches().RecordResult(context.Background(), 50, 3, 3, true); err != nil {
		t.Fatalf("RecordResult edited: %v", err)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_Get_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, league_id, week, home_team_id, away_team_id,
			        home_goals, away_goals, played_at, edited_at
			   FROM matches WHERE id = ?`).
		WithArgs(int64(404)).
		WillReturnError(sqlNoRows())

	_, err := store.Matches().Get(context.Background(), 404)
	if !errors.Is(err, domain.ErrMatchNotFound) {
		t.Errorf("expected ErrMatchNotFound, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_Get_WithGoals(t *testing.T) {
	store, mock := newTestStore(t)
	played := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT id, league_id, week, home_team_id, away_team_id,
			        home_goals, away_goals, played_at, edited_at
			   FROM matches WHERE id = ?`).
		WithArgs(int64(50)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "league_id", "week", "home_team_id", "away_team_id",
			"home_goals", "away_goals", "played_at", "edited_at",
		}).AddRow(int64(50), int64(1), 2, int64(10), int64(11), int64(3), int64(1), played, nil))

	m, err := store.Matches().Get(context.Background(), 50)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.HomeGoals == nil || *m.HomeGoals != 3 || m.AwayGoals == nil || *m.AwayGoals != 1 {
		t.Errorf("goals = %v / %v", m.HomeGoals, m.AwayGoals)
	}
	if m.PlayedAt == nil || m.EditedAt != nil {
		t.Errorf("played_at=%v edited_at=%v", m.PlayedAt, m.EditedAt)
	}
	assertExpectations(t, mock)
}

func TestMatchRepo_List(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, league_id, week, home_team_id, away_team_id,
			        home_goals, away_goals, played_at, edited_at
			   FROM matches WHERE league_id = ? ORDER BY week ASC, id ASC`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "league_id", "week", "home_team_id", "away_team_id",
			"home_goals", "away_goals", "played_at", "edited_at",
		}).AddRow(int64(1), int64(1), 1, int64(10), int64(11), nil, nil, nil, nil))

	got, err := store.Matches().List(context.Background(), 1)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].HomeGoals != nil {
		t.Errorf("got = %+v", got)
	}
	assertExpectations(t, mock)
}
