package domain

import "time"

// played returns m with the given goals filled in and a non-nil PlayedAt.
func played(m Match, h, a int) Match {
	hg, ag := h, a
	m.HomeGoals = &hg
	m.AwayGoals = &ag
	t := time.Time{}
	m.PlayedAt = &t
	return m
}

// fourTeams returns a stable four-team fixture used across tests.
func fourTeams() []Team {
	return []Team{
		{ID: 1, Name: "Alpha", Strength: 80},
		{ID: 2, Name: "Bravo", Strength: 75},
		{ID: 3, Name: "Charlie", Strength: 70},
		{ID: 4, Name: "Delta", Strength: 65},
	}
}
