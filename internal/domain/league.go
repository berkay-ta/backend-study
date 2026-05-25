package domain

import "time"

const (
	LeagueSize        = 4
	PredictionMinWeek = 4
)

func MatchesPerWeek(teamCount int) int {
	return teamCount / 2
}

func TotalWeeksForTeamCount(teamCount int) int {
	if teamCount < 2 {
		return 0
	}
	roundsPerLeg := teamCount - 1
	if teamCount%2 != 0 {
		roundsPerLeg = teamCount
	}
	return roundsPerLeg * 2
}

func TotalMatchesForTeamCount(teamCount int) int {
	if teamCount < 2 {
		return 0
	}
	return teamCount * (teamCount - 1)
}

type League struct {
	ID          int64
	Name        string
	CurrentWeek int
	TotalWeeks  int
	Version     int64
	CreatedAt   time.Time
}

func (l League) IsComplete() bool { return l.CurrentWeek > l.TotalWeeks }

// CurrentWeek is the next week to play, so >4 means 4 weeks already played.
func (l League) PredictionsOpen() bool { return l.CurrentWeek > PredictionMinWeek }
