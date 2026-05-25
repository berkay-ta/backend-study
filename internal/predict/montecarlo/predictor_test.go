package montecarlo_test

import (
	"context"
	"testing"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/predict/montecarlo"
)

// Determinism is the headline contract: identical inputs must yield identical
// PredictionResults so reviewers can run the same Postman request twice and
// compare bytes if they want to.
func TestPredictor_Deterministic(t *testing.T) {
	t.Parallel()

	in := fixtureInput(7)
	p := montecarlo.New(2_000)

	a, err := p.Predict(context.Background(), in)
	if err != nil {
		t.Fatalf("first predict: %v", err)
	}
	b, err := p.Predict(context.Background(), in)
	if err != nil {
		t.Fatalf("second predict: %v", err)
	}
	if len(a.Entries) != len(b.Entries) {
		t.Fatalf("entry counts differ: %d vs %d", len(a.Entries), len(b.Entries))
	}
	for i := range a.Entries {
		if a.Entries[i] != b.Entries[i] {
			t.Errorf("entry %d differs: %+v vs %+v", i, a.Entries[i], b.Entries[i])
		}
	}
}

// Different league_version means a different seed and different (but still valid) output.
func TestPredictor_VersionChangesOutput(t *testing.T) {
	t.Parallel()

	in1 := fixtureInput(1)
	in2 := fixtureInput(2)

	p := montecarlo.New(1_500)

	r1, err := p.Predict(context.Background(), in1)
	if err != nil {
		t.Fatalf("predict v1: %v", err)
	}
	r2, err := p.Predict(context.Background(), in2)
	if err != nil {
		t.Fatalf("predict v2: %v", err)
	}
	identical := true
	for i := range r1.Entries {
		if r1.Entries[i].ChampionshipPct != r2.Entries[i].ChampionshipPct {
			identical = false
			break
		}
	}
	if identical {
		t.Error("expected different percentages across league versions, got identical")
	}
}

// LSP contract: every league team present exactly once, projected table fields
// populated, percentages sum to exactly 100, expected_points >= 0.
func TestPredictor_Contract(t *testing.T) {
	t.Parallel()

	in := fixtureInput(3)
	p := montecarlo.New(1_000)

	res, err := p.Predict(context.Background(), in)
	if err != nil {
		t.Fatalf("predict: %v", err)
	}
	if len(res.Entries) != len(in.Teams) {
		t.Fatalf("expected %d entries, got %d", len(in.Teams), len(res.Entries))
	}
	seen := map[int64]bool{}
	var sum float64
	for _, e := range res.Entries {
		if e.ProjectedPosition < 1 || e.ProjectedPosition > len(in.Teams) {
			t.Errorf("team %d invalid projected_position: %d", e.TeamID, e.ProjectedPosition)
		}
		if e.Played != in.League.TotalWeeks {
			t.Errorf("team %d played=%d, expected %d", e.TeamID, e.Played, in.League.TotalWeeks)
		}
		if e.AveragePosition < 1 || e.AveragePosition > float64(len(in.Teams)) {
			t.Errorf("team %d invalid average_position: %.2f", e.TeamID, e.AveragePosition)
		}
		if seen[e.TeamID] {
			t.Errorf("team %d appeared twice", e.TeamID)
		}
		seen[e.TeamID] = true
		if e.ChampionshipPct < 0 || e.ChampionshipPct > 100 {
			t.Errorf("team %d pct out of range: %.2f", e.TeamID, e.ChampionshipPct)
		}
		if e.ExpectedPoints < 0 {
			t.Errorf("team %d negative expected_points: %.2f", e.TeamID, e.ExpectedPoints)
		}
		if e.ExpectedGoalsFor < 0 || e.ExpectedGoalsAgainst < 0 {
			t.Errorf("team %d negative expected goals: %.2f %.2f", e.TeamID, e.ExpectedGoalsFor, e.ExpectedGoalsAgainst)
		}
		sum += e.ChampionshipPct
	}
	if sum != 100.0 {
		t.Errorf("expected sum=100.00 (after normalisation), got %.4f", sum)
	}
}

// fixtureInput builds a minimal but realistic PredictionInput for tests.
// Strengths are spread so the leader is heavily favoured but upsets remain
// possible, exercising both ChampionshipPct concentration and the tail.
func fixtureInput(version int64) app.PredictionInput {
	teams := []domain.Team{
		{ID: 1, Name: "Alpha", Strength: 90},
		{ID: 2, Name: "Bravo", Strength: 70},
		{ID: 3, Name: "Charlie", Strength: 60},
		{ID: 4, Name: "Delta", Strength: 50},
	}
	played := []domain.Match{
		mustMatch(1, 1, 1, 2, 2, 1),
		mustMatch(2, 1, 3, 4, 1, 0),
		mustMatch(3, 2, 1, 3, 3, 0),
		mustMatch(4, 2, 2, 4, 2, 2),
		mustMatch(5, 3, 1, 4, 1, 1),
		mustMatch(6, 3, 2, 3, 1, 0),
		mustMatch(7, 4, 4, 1, 0, 3),
		mustMatch(8, 4, 3, 2, 1, 2),
	}
	remaining := []domain.Match{
		{ID: 9, LeagueID: 1, Week: 5, HomeTeamID: 1, AwayTeamID: 2},
		{ID: 10, LeagueID: 1, Week: 5, HomeTeamID: 3, AwayTeamID: 4},
		{ID: 11, LeagueID: 1, Week: 6, HomeTeamID: 2, AwayTeamID: 1},
		{ID: 12, LeagueID: 1, Week: 6, HomeTeamID: 4, AwayTeamID: 3},
	}
	league := domain.League{ID: 1, Name: "Test", CurrentWeek: 5, TotalWeeks: 6, Version: version}
	snapshot := (domain.StandingCalculator{}).Compute(teams, played)
	return app.PredictionInput{
		League:        league,
		Teams:         teams,
		PlayedMatches: played,
		Remaining:     remaining,
		Snapshot:      snapshot,
	}
}

func mustMatch(id, week int64, home, away int64, hg, ag int) domain.Match {
	return domain.Match{
		ID: id, LeagueID: 1, Week: int(week),
		HomeTeamID: home, AwayTeamID: away,
		HomeGoals: ptrInt(hg), AwayGoals: ptrInt(ag),
	}
}

func ptrInt(v int) *int { return &v }
