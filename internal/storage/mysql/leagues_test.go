package mysql

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/iberkayC/case1back/internal/domain"
)

func TestLeagueRepo_Create(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(`INSERT INTO leagues (name, current_week, total_weeks, version) VALUES (?, 1, ?, 0)`).
		WithArgs("Test League", 6).
		WillReturnResult(sqlmock.NewResult(7, 1))

	mock.ExpectExec("INSERT INTO teams (league_id, name, strength) VALUES (?, ?, ?), (?, ?, ?)").
		WithArgs(int64(7), "Alpha", uint8(90), int64(7), "Bravo", uint8(70)).
		WillReturnResult(sqlmock.NewResult(11, 2))

	mock.ExpectQuery(`SELECT id, name FROM teams WHERE league_id = ?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
			AddRow(int64(11), "Alpha").
			AddRow(int64(12), "Bravo"))

	mock.ExpectQuery(`SELECT id, name, current_week, total_weeks, version, created_at
			   FROM leagues WHERE id = ?`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "current_week", "total_weeks", "version", "created_at"}).
			AddRow(int64(7), "Test League", 1, 6, int64(0), now))

	teams := []domain.Team{
		{Name: "Alpha", Strength: 90},
		{Name: "Bravo", Strength: 70},
	}
	league, gotTeams, err := store.Leagues().Create(context.Background(), "Test League", 6, teams)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if league.ID != 7 || league.Name != "Test League" {
		t.Errorf("league = %+v", league)
	}
	if len(gotTeams) != 2 || gotTeams[0].ID != 11 || gotTeams[1].ID != 12 {
		t.Errorf("teams = %+v", gotTeams)
	}
	assertExpectations(t, mock)
}

func TestLeagueRepo_Get_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, name, current_week, total_weeks, version, created_at
			   FROM leagues WHERE id = ?`).
		WithArgs(int64(99)).
		WillReturnError(sqlNoRows())

	_, err := store.Leagues().Get(context.Background(), 99)
	if !errors.Is(err, domain.ErrLeagueNotFound) {
		t.Errorf("expected ErrLeagueNotFound, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestLeagueRepo_GetForUpdate(t *testing.T) {
	store, mock := newTestStore(t)
	now := time.Now().UTC()

	mock.ExpectQuery(`SELECT id, name, current_week, total_weeks, version, created_at
			   FROM leagues
			  WHERE id = ?
			  FOR UPDATE`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "current_week", "total_weeks", "version", "created_at"}).
			AddRow(int64(3), "Locked", 4, 6, int64(2), now))

	got, err := store.Leagues().GetForUpdate(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetForUpdate: %v", err)
	}
	if got.ID != 3 || got.Version != 2 {
		t.Errorf("got = %+v", got)
	}
	assertExpectations(t, mock)
}

func TestLeagueRepo_Delete_NotFound(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`DELETE FROM leagues WHERE id = ?`).
		WithArgs(int64(13)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.Leagues().Delete(context.Background(), 13)
	if !errors.Is(err, domain.ErrLeagueNotFound) {
		t.Errorf("expected ErrLeagueNotFound, got %v", err)
	}
	assertExpectations(t, mock)
}

func TestLeagueRepo_BumpVersion(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectExec(`UPDATE leagues SET version = version + 1 WHERE id = ?`).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT version FROM leagues WHERE id = ?`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(int64(8)))

	v, err := store.Leagues().BumpVersion(context.Background(), 5)
	if err != nil {
		t.Fatalf("BumpVersion: %v", err)
	}
	if v != 8 {
		t.Errorf("version = %d, want 8", v)
	}
	assertExpectations(t, mock)
}

func TestLeagueRepo_GetTeams(t *testing.T) {
	store, mock := newTestStore(t)

	mock.ExpectQuery(`SELECT id, league_id, name, strength FROM teams WHERE league_id = ? ORDER BY name ASC`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "league_id", "name", "strength"}).
			AddRow(int64(10), int64(1), "Alpha", 90).
			AddRow(int64(11), int64(1), "Bravo", 70))

	got, err := store.Leagues().GetTeams(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetTeams: %v", err)
	}
	if len(got) != 2 || got[0].Name != "Alpha" || got[1].Strength != 70 {
		t.Errorf("teams = %+v", got)
	}
	assertExpectations(t, mock)
}
