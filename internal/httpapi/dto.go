package httpapi

import (
	"fmt"
	"sort"
	"time"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

type createLeagueReq struct {
	Name  string         `json:"name"`
	Teams []teamInputDTO `json:"teams"`
}

type teamInputDTO struct {
	Name     string `json:"name"`
	Strength int    `json:"strength"`
}

func (r createLeagueReq) toService() app.CreateLeagueInput {
	in := app.CreateLeagueInput{Name: r.Name, Teams: make([]app.TeamInput, len(r.Teams))}
	for i, t := range r.Teams {
		in.Teams[i] = app.TeamInput{Name: t.Name, Strength: t.Strength}
	}
	return in
}

// editMatchReq carries the two required match scores. Pointers let validate
// tell an omitted field apart from a legitimate zero score.
type editMatchReq struct {
	HomeGoals *int `json:"home_goals"`
	AwayGoals *int `json:"away_goals"`
}

func (r editMatchReq) validate() error {
	var fields []apperror.FieldError
	if r.HomeGoals == nil {
		fields = append(fields, apperror.FieldError{Field: "home_goals", Message: "is required"})
	}
	if r.AwayGoals == nil {
		fields = append(fields, apperror.FieldError{Field: "away_goals", Message: "is required"})
	}
	if len(fields) > 0 {
		return apperror.ValidationFailed(fields...)
	}
	return nil
}

type whatIfReq struct {
	Overrides []whatIfOverrideDTO `json:"overrides"`
	Strategy  string              `json:"strategy"`
}

// whatIfOverrideDTO carries one required score override. Pointers let validate
// tell an omitted field apart from a legitimate zero value.
type whatIfOverrideDTO struct {
	MatchID   *int64 `json:"match_id"`
	HomeGoals *int   `json:"home_goals"`
	AwayGoals *int   `json:"away_goals"`
}

func (r whatIfReq) validate() error {
	var fields []apperror.FieldError
	for i, o := range r.Overrides {
		if o.MatchID == nil {
			fields = append(fields, apperror.FieldError{Field: fmt.Sprintf("overrides[%d].match_id", i), Message: "is required"})
		}
		if o.HomeGoals == nil {
			fields = append(fields, apperror.FieldError{Field: fmt.Sprintf("overrides[%d].home_goals", i), Message: "is required"})
		}
		if o.AwayGoals == nil {
			fields = append(fields, apperror.FieldError{Field: fmt.Sprintf("overrides[%d].away_goals", i), Message: "is required"})
		}
	}
	if len(fields) > 0 {
		return apperror.ValidationFailed(fields...)
	}
	return nil
}

// toService assumes validate has already run, so every override field is set.
func (r whatIfReq) toService(leagueID int64) app.WhatIfInput {
	in := app.WhatIfInput{
		LeagueID:  leagueID,
		Strategy:  domain.Strategy(r.Strategy),
		Overrides: make([]domain.MatchOverride, len(r.Overrides)),
	}
	for i, o := range r.Overrides {
		in.Overrides[i] = domain.MatchOverride{MatchID: *o.MatchID, HomeGoals: *o.HomeGoals, AwayGoals: *o.AwayGoals}
	}
	return in
}

type leagueDTO struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	CurrentWeek int       `json:"current_week"`
	TotalWeeks  int       `json:"total_weeks"`
	Version     int64     `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	Complete    bool      `json:"complete"`
}

func toLeagueDTO(l domain.League) leagueDTO {
	return leagueDTO{
		ID: l.ID, Name: l.Name,
		CurrentWeek: l.CurrentWeek, TotalWeeks: l.TotalWeeks,
		Version: l.Version, CreatedAt: l.CreatedAt,
		Complete: l.IsComplete(),
	}
}

type teamDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Strength int    `json:"strength"`
}

func toTeamDTO(t domain.Team) teamDTO {
	return teamDTO{ID: t.ID, Name: t.Name, Strength: t.Strength}
}

func toTeamDTOs(ts []domain.Team) []teamDTO {
	out := make([]teamDTO, len(ts))
	for i, t := range ts {
		out[i] = toTeamDTO(t)
	}
	return out
}

type matchDTO struct {
	ID         int64      `json:"id"`
	LeagueID   int64      `json:"league_id"`
	Week       int        `json:"week"`
	HomeTeamID int64      `json:"home_team_id"`
	AwayTeamID int64      `json:"away_team_id"`
	HomeGoals  *int       `json:"home_goals"`
	AwayGoals  *int       `json:"away_goals"`
	PlayedAt   *time.Time `json:"played_at"`
	EditedAt   *time.Time `json:"edited_at"`
	Played     bool       `json:"played"`
}

func toMatchDTO(m domain.Match) matchDTO {
	return matchDTO{
		ID: m.ID, LeagueID: m.LeagueID, Week: m.Week,
		HomeTeamID: m.HomeTeamID, AwayTeamID: m.AwayTeamID,
		HomeGoals: m.HomeGoals, AwayGoals: m.AwayGoals,
		PlayedAt: m.PlayedAt, EditedAt: m.EditedAt,
		Played: m.Played(),
	}
}

func toMatchDTOs(ms []domain.Match) []matchDTO {
	out := make([]matchDTO, len(ms))
	for i, m := range ms {
		out[i] = toMatchDTO(m)
	}
	return out
}

type standingRowDTO struct {
	Position     int    `json:"position"`
	TeamID       int64  `json:"team_id"`
	TeamName     string `json:"team_name"`
	Played       int    `json:"played"`
	Won          int    `json:"won"`
	Drawn        int    `json:"drawn"`
	Lost         int    `json:"lost"`
	GoalsFor     int    `json:"goals_for"`
	GoalsAgainst int    `json:"goals_against"`
	GoalDiff     int    `json:"goal_diff"`
	Points       int    `json:"points"`
}

func toStandingRowDTO(r domain.StandingRow) standingRowDTO {
	return standingRowDTO{
		Position: r.Position, TeamID: r.TeamID, TeamName: r.TeamName,
		Played: r.Played, Won: r.Won, Drawn: r.Drawn, Lost: r.Lost,
		GoalsFor: r.GoalsFor, GoalsAgainst: r.GoalsAgainst,
		GoalDiff: r.GoalDiff, Points: r.Points,
	}
}

func toStandingRowDTOs(rs []domain.StandingRow) []standingRowDTO {
	out := make([]standingRowDTO, len(rs))
	for i, r := range rs {
		out[i] = toStandingRowDTO(r)
	}
	return out
}

type predictionEntryDTO struct {
	ProjectedPosition    int     `json:"projected_position"`
	TeamID               int64   `json:"team_id"`
	TeamName             string  `json:"team_name"`
	ChampionshipPct      float64 `json:"championship_pct"`
	AveragePosition      float64 `json:"average_position"`
	Played               int     `json:"played"`
	ExpectedWon          float64 `json:"expected_won"`
	ExpectedDrawn        float64 `json:"expected_drawn"`
	ExpectedLost         float64 `json:"expected_lost"`
	ExpectedGoalsFor     float64 `json:"expected_goals_for"`
	ExpectedGoalsAgainst float64 `json:"expected_goals_against"`
	ExpectedGoalDiff     float64 `json:"expected_goal_diff"`
	ExpectedPoints       float64 `json:"expected_points"`
}

type predictionResultDTO struct {
	Strategy       domain.Strategy      `json:"strategy"`
	ProjectedTable []predictionEntryDTO `json:"projected_table"`
	Entries        []predictionEntryDTO `json:"entries"`
	Notes          string               `json:"notes,omitempty"`
}

func toPredictionResultDTO(r domain.PredictionResult) predictionResultDTO {
	out := predictionResultDTO{
		Strategy:       r.Strategy,
		Notes:          r.Notes,
		ProjectedTable: make([]predictionEntryDTO, len(r.Entries)),
		Entries:        make([]predictionEntryDTO, len(r.Entries)),
	}
	for i, e := range r.Entries {
		dto := predictionEntryDTO{
			ProjectedPosition:    e.ProjectedPosition,
			TeamID:               e.TeamID,
			TeamName:             e.TeamName,
			ChampionshipPct:      e.ChampionshipPct,
			AveragePosition:      e.AveragePosition,
			Played:               e.Played,
			ExpectedWon:          e.ExpectedWon,
			ExpectedDrawn:        e.ExpectedDrawn,
			ExpectedLost:         e.ExpectedLost,
			ExpectedGoalsFor:     e.ExpectedGoalsFor,
			ExpectedGoalsAgainst: e.ExpectedGoalsAgainst,
			ExpectedGoalDiff:     e.ExpectedGoalDiff,
			ExpectedPoints:       e.ExpectedPoints,
		}
		out.ProjectedTable[i] = dto
		out.Entries[i] = dto
	}
	return out
}

type predictionRunDTO struct {
	ID            int64               `json:"id"`
	LeagueID      int64               `json:"league_id"`
	Strategy      domain.Strategy     `json:"strategy"`
	SnapshotID    int64               `json:"snapshot_id"`
	LeagueVersion int64               `json:"league_version"`
	CreatedAt     time.Time           `json:"created_at"`
	Stale         bool                `json:"stale"`
	Result        predictionResultDTO `json:"result"`
}

func toPredictionRunDTO(r domain.PredictionRun) predictionRunDTO {
	return predictionRunDTO{
		ID: r.ID, LeagueID: r.LeagueID, Strategy: r.Strategy,
		SnapshotID: r.SnapshotID, LeagueVersion: r.LeagueVersion,
		CreatedAt: r.CreatedAt, Stale: r.Stale,
		Result: toPredictionResultDTO(r.Result),
	}
}

func toPredictionRunDTOs(rs []domain.PredictionRun) []predictionRunDTO {
	out := make([]predictionRunDTO, len(rs))
	for i, r := range rs {
		out[i] = toPredictionRunDTO(r)
	}
	return out
}

type createLeagueRespDTO struct {
	League   leagueDTO  `json:"league"`
	Teams    []teamDTO  `json:"teams"`
	Fixtures []matchDTO `json:"fixtures"`
}

type editMatchRespDTO struct {
	Match     matchDTO         `json:"match"`
	Standings []standingRowDTO `json:"standings"`
}

type weekPlayedDTO struct {
	Week    int        `json:"week"`
	Matches []matchDTO `json:"matches"`
}

type playWeekRespDTO struct {
	Week      int              `json:"week"`
	Matches   []matchDTO       `json:"matches"`
	Standings []standingRowDTO `json:"standings"`
	League    leagueDTO        `json:"league"`
}

type playAllRespDTO struct {
	Weeks     []weekPlayedDTO  `json:"weeks"`
	Standings []standingRowDTO `json:"standings"`
	League    leagueDTO        `json:"league"`
}

type whatIfRespDTO struct {
	Standings  []standingRowDTO     `json:"standings"`
	Prediction *predictionResultDTO `json:"prediction,omitempty"`
}

type fixturesByWeekDTO struct {
	Week    int        `json:"week"`
	Matches []matchDTO `json:"matches"`
}

func toFixturesByWeek(ms []domain.Match) []fixturesByWeekDTO {
	byWeek := map[int][]matchDTO{}
	for _, m := range ms {
		byWeek[m.Week] = append(byWeek[m.Week], toMatchDTO(m))
	}
	weeks := make([]int, 0, len(byWeek))
	for w := range byWeek {
		weeks = append(weeks, w)
	}
	sort.Ints(weeks)
	out := make([]fixturesByWeekDTO, 0, len(byWeek))
	for _, w := range weeks {
		out = append(out, fixturesByWeekDTO{Week: w, Matches: byWeek[w]})
	}
	return out
}
