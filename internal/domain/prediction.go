package domain

import (
	"sort"
	"time"
)

type Strategy string

const (
	StrategyMonteCarlo Strategy = "monte_carlo"
	StrategyAIAnalyst  Strategy = "ai_analyst"
)

func KnownStrategy(s Strategy) bool {
	switch s {
	case StrategyMonteCarlo, StrategyAIAnalyst:
		return true
	}
	return false
}

type PredictionEntry struct {
	ProjectedPosition    int
	TeamID               int64
	TeamName             string
	ChampionshipPct      float64
	AveragePosition      float64
	Played               int
	ExpectedWon          float64
	ExpectedDrawn        float64
	ExpectedLost         float64
	ExpectedGoalsFor     float64
	ExpectedGoalsAgainst float64
	ExpectedGoalDiff     float64
	ExpectedPoints       float64
}

type PredictionResult struct {
	Strategy Strategy
	Entries  []PredictionEntry
	Notes    string
}

// SortPredictionEntries orders prediction entries as an estimated final table
// and rewrites ProjectedPosition to match that order.
func SortPredictionEntries(entries []PredictionEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.ExpectedPoints != b.ExpectedPoints {
			return a.ExpectedPoints > b.ExpectedPoints
		}
		if a.ChampionshipPct != b.ChampionshipPct {
			return a.ChampionshipPct > b.ChampionshipPct
		}
		if a.ExpectedGoalDiff != b.ExpectedGoalDiff {
			return a.ExpectedGoalDiff > b.ExpectedGoalDiff
		}
		return a.TeamName < b.TeamName
	})
	for i := range entries {
		entries[i].ProjectedPosition = i + 1
	}
}

type PredictionRun struct {
	ID            int64
	LeagueID      int64
	Strategy      Strategy
	SnapshotID    int64
	LeagueVersion int64
	CreatedAt     time.Time
	Stale         bool
	Result        PredictionResult
	ParamsJSON    []byte
}
