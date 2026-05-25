package domain

type FixtureGenerator struct{}

// Generate builds a double round-robin schedule. It's deterministic, the same
// teams in the same order always yield the same fixtures, so a league setup is
// reproducible.
func (FixtureGenerator) Generate(leagueID int64, teams []Team) []Match {
	if len(teams) != LeagueSize {
		return nil
	}

	rounds := firstLegRounds(len(teams))
	matches := make([]Match, 0, TotalMatchesForTeamCount(len(teams)))
	week := 1
	for _, round := range rounds {
		for _, p := range round {
			matches = append(matches, Match{
				LeagueID: leagueID, Week: week,
				HomeTeamID: teams[p.home].ID, AwayTeamID: teams[p.away].ID,
			})
		}
		week++
	}
	for _, round := range rounds {
		for _, p := range round {
			matches = append(matches, Match{
				LeagueID: leagueID, Week: week,
				HomeTeamID: teams[p.away].ID, AwayTeamID: teams[p.home].ID,
			})
		}
		week++
	}
	return matches
}

type fixturePair struct{ home, away int }

func firstLegRounds(teamCount int) [][]fixturePair {
	const bye = -1
	slots := make([]int, teamCount)
	for i := range slots {
		slots[i] = i
	}
	if teamCount%2 != 0 {
		// Generalizing league size, even though it's 4 in the study.
		slots = append(slots, bye)
	}

	rounds := make([][]fixturePair, 0, len(slots)-1)
	for round := 0; round < len(slots)-1; round++ {
		pairs := make([]fixturePair, 0, len(slots)/2)
		for i := 0; i < len(slots)/2; i++ {
			a, b := slots[i], slots[len(slots)-1-i]
			if a == bye || b == bye {
				continue
			}
			if (round+i)%2 == 0 {
				pairs = append(pairs, fixturePair{home: a, away: b})
			} else {
				pairs = append(pairs, fixturePair{home: b, away: a})
			}
		}
		rounds = append(rounds, pairs)
		rotateSlots(slots)
	}
	return rounds
}

func rotateSlots(slots []int) {
	if len(slots) <= 2 {
		return
	}
	last := slots[len(slots)-1]
	copy(slots[2:], slots[1:len(slots)-1])
	slots[1] = last
}
