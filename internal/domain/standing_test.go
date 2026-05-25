package domain

import (
	"testing"
)

func TestStandingCalculator_PremierLeagueScoring(t *testing.T) {
	teams := fourTeams()
	matches := []Match{
		played(Match{ID: 1, Week: 1, HomeTeamID: 1, AwayTeamID: 2}, 2, 1), // Alpha beats Bravo
		played(Match{ID: 2, Week: 1, HomeTeamID: 3, AwayTeamID: 4}, 1, 0), // Charlie beats Delta
		played(Match{ID: 3, Week: 2, HomeTeamID: 1, AwayTeamID: 3}, 3, 1), // Alpha beats Charlie
		played(Match{ID: 4, Week: 2, HomeTeamID: 2, AwayTeamID: 4}, 2, 2), // Bravo draws Delta
	}

	rows := StandingCalculator{}.Compute(teams, matches)

	if got, want := len(rows), 4; got != want {
		t.Fatalf("rows: got %d want %d", got, want)
	}

	want := []struct {
		name   string
		points int
		gd     int
		gf     int
	}{
		{"Alpha", 6, 3, 5},
		{"Charlie", 3, -1, 2},
		{"Bravo", 1, -1, 3},
		{"Delta", 1, -1, 2},
	}
	for i, w := range want {
		got := rows[i]
		if got.TeamName != w.name {
			t.Errorf("pos %d name: got %q want %q", i+1, got.TeamName, w.name)
		}
		if got.Points != w.points {
			t.Errorf("%s points: got %d want %d", w.name, got.Points, w.points)
		}
		if got.GoalDiff != w.gd {
			t.Errorf("%s GD: got %d want %d", w.name, got.GoalDiff, w.gd)
		}
		if got.GoalsFor != w.gf {
			t.Errorf("%s GF: got %d want %d", w.name, got.GoalsFor, w.gf)
		}
		if got.Position != i+1 {
			t.Errorf("%s position: got %d want %d", w.name, got.Position, i+1)
		}
	}
}

func TestStandingCalculator_IgnoresUnplayed(t *testing.T) {
	teams := fourTeams()
	matches := []Match{
		played(Match{ID: 1, HomeTeamID: 1, AwayTeamID: 2}, 1, 0),
		{ID: 2, HomeTeamID: 3, AwayTeamID: 4}, // unplayed
	}
	rows := StandingCalculator{}.Compute(teams, matches)

	if len(rows) != 4 {
		t.Fatalf("rows: got %d want 4", len(rows))
	}
	playedCount := 0
	for _, r := range rows {
		playedCount += r.Played
	}
	if playedCount != 2 {
		t.Fatalf("sum of Played: got %d want 2 (1 match, 2 played slots)", playedCount)
	}
}

func TestTieBreak_PrefersGoalDifferenceThenGF(t *testing.T) {
	// Two rows with equal points; A has better GD, then equal GD but better GF.
	a := StandingRow{TeamName: "A", Points: 6, GoalDiff: 5, GoalsFor: 7}
	b := StandingRow{TeamName: "B", Points: 6, GoalDiff: 2, GoalsFor: 9}
	if !standingLess(a, b) {
		t.Fatalf("expected A before B (better GD)")
	}
	a.GoalDiff = 2
	if standingLess(a, b) {
		t.Fatalf("expected B before A (equal GD, better GF)")
	}
}

// TestTieBreak_CycleResolvesByNameDeterministically: three teams dead level on
// points, goal difference, and goals scored with a head-to-head cycle
// (Alpha>Bravo>Charlie>Alpha). Premier League rules use no head-to-head, so the
// order falls to name and must be identical on every run; Compute builds rows
// from a randomized map iteration, so a non-transitive comparator would yield
// different tables across runs.
func TestTieBreak_CycleResolvesByNameDeterministically(t *testing.T) {
	teams := []Team{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Bravo"},
		{ID: 3, Name: "Charlie"},
	}
	matches := []Match{
		played(Match{ID: 1, HomeTeamID: 1, AwayTeamID: 2}, 1, 0), // Alpha beat Bravo
		played(Match{ID: 2, HomeTeamID: 2, AwayTeamID: 3}, 1, 0), // Bravo beat Charlie
		played(Match{ID: 3, HomeTeamID: 3, AwayTeamID: 1}, 1, 0), // Charlie beat Alpha
	}
	// Each team: 1W 1L = 3 pts, GD 0, GF 1. Fully level, so name decides.
	want := []string{"Alpha", "Bravo", "Charlie"}

	for trial := 0; trial < 50; trial++ {
		rows := StandingCalculator{}.Compute(teams, matches)
		for i, name := range want {
			if rows[i].TeamName != name {
				t.Fatalf("trial %d position %d: got %q, want %q", trial, i+1, rows[i].TeamName, name)
			}
		}
	}
}

func TestTieBreak_AlphabeticalFinalFallback(t *testing.T) {
	a := StandingRow{TeamID: 1, TeamName: "Alpha", Points: 0, GoalDiff: 0, GoalsFor: 0}
	b := StandingRow{TeamID: 2, TeamName: "Bravo", Points: 0, GoalDiff: 0, GoalsFor: 0}
	if !standingLess(a, b) {
		t.Fatalf("expected Alpha before Bravo as final tie-breaker")
	}
}
