package domain

type Team struct {
	ID       int64
	LeagueID int64
	Name     string
	Strength int
}

func TeamsByID(teams []Team) map[int64]Team {
	out := make(map[int64]Team, len(teams))
	for _, t := range teams {
		out[t.ID] = t
	}
	return out
}
