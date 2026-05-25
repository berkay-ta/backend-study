package domain

import "time"

type Match struct {
	ID         int64
	LeagueID   int64
	Week       int
	HomeTeamID int64
	AwayTeamID int64

	HomeGoals *int
	AwayGoals *int

	PlayedAt *time.Time
	EditedAt *time.Time
}

func (m Match) Played() bool { return m.HomeGoals != nil && m.AwayGoals != nil }

type MatchOverride struct {
	MatchID   int64
	HomeGoals int
	AwayGoals int
}
