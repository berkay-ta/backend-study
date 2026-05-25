package domain

import (
	"sort"
	"strings"
)

const (
	PointsWin  = 3
	PointsDraw = 1
	PointsLoss = 0
)

type StandingRow struct {
	Position     int
	TeamID       int64
	TeamName     string
	Played       int
	Won          int
	Drawn        int
	Lost         int
	GoalsFor     int
	GoalsAgainst int
	GoalDiff     int
	Points       int
}

type StandingCalculator struct{}

// Compute returns the rows for all teams in tie-break order. Unplayed matches
// are ignored.
func (StandingCalculator) Compute(teams []Team, matches []Match) []StandingRow {
	byTeam := make(map[int64]*StandingRow, len(teams))
	for _, t := range teams {
		byTeam[t.ID] = &StandingRow{TeamID: t.ID, TeamName: t.Name}
	}

	for _, m := range matches {
		if !m.Played() {
			continue
		}
		home, away := byTeam[m.HomeTeamID], byTeam[m.AwayTeamID]
		if home == nil || away == nil {
			continue
		}
		home.Played++
		away.Played++
		home.GoalsFor += *m.HomeGoals
		home.GoalsAgainst += *m.AwayGoals
		away.GoalsFor += *m.AwayGoals
		away.GoalsAgainst += *m.HomeGoals
		switch {
		case *m.HomeGoals > *m.AwayGoals:
			home.Won++
			home.Points += PointsWin
			away.Lost++
		case *m.HomeGoals < *m.AwayGoals:
			away.Won++
			away.Points += PointsWin
			home.Lost++
		default:
			home.Drawn++
			away.Drawn++
			home.Points += PointsDraw
			away.Points += PointsDraw
		}
	}

	rows := make([]StandingRow, 0, len(byTeam))
	for _, r := range byTeam {
		r.GoalDiff = r.GoalsFor - r.GoalsAgainst
		rows = append(rows, *r)
	}

	sort.SliceStable(rows, func(i, j int) bool { return standingLess(rows[i], rows[j]) })

	for i := range rows {
		rows[i].Position = i + 1
	}
	return rows
}

// standingLess orders the table (Premier League criteria): points, goal
// difference, goals scored, then name and ID as tie-breakers for a strict
// total order. Used for both the sort and championship credit.
func standingLess(a, b StandingRow) bool {
	if a.Points != b.Points {
		return a.Points > b.Points
	}
	if a.GoalDiff != b.GoalDiff {
		return a.GoalDiff > b.GoalDiff
	}
	if a.GoalsFor != b.GoalsFor {
		return a.GoalsFor > b.GoalsFor
	}
	// Sporting criteria exhausted: fall back to lexicographic name, then ID,
	// purely to keep the order deterministic.
	if c := strings.Compare(a.TeamName, b.TeamName); c != 0 {
		return c < 0
	}
	return a.TeamID < b.TeamID
}

// ChampionShares awards title credit to the leader(s) under standingLess. With
// the name/ID fallback a tie resolves to a single deterministic champion, so
// this normally returns one team with a 1.0 share.
func ChampionShares(rows []StandingRow) map[int64]float64 {
	out := map[int64]float64{}
	if len(rows) == 0 {
		return out
	}
	leaders := 0
	for _, r := range rows {
		// Equal under the ordering: neither sorts before the other.
		if standingLess(rows[0], r) || standingLess(r, rows[0]) {
			break
		}
		leaders++
	}
	if leaders == 0 {
		leaders = 1
	}
	share := 1.0 / float64(leaders)
	for i := 0; i < leaders; i++ {
		out[rows[i].TeamID] = share
	}
	return out
}
