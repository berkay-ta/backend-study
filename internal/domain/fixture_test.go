package domain

import (
	"fmt"
	"testing"
)

func TestFixtureGenerator_DoubleRoundRobinShape(t *testing.T) {
	teams := fourTeams()
	matches := FixtureGenerator{}.Generate(17, teams)
	totalWeeks := TotalWeeksForTeamCount(len(teams))
	matchesPerWeek := MatchesPerWeek(len(teams))

	if got, want := len(matches), TotalMatchesForTeamCount(len(teams)); got != want {
		t.Fatalf("match count: got %d want %d", got, want)
	}

	// Each week has exactly the expected number of games.
	weekCounts := map[int]int{}
	for _, m := range matches {
		if m.LeagueID != 17 {
			t.Errorf("LeagueID propagated wrong: got %d want 17", m.LeagueID)
		}
		weekCounts[m.Week]++
	}
	for w := 1; w <= totalWeeks; w++ {
		if weekCounts[w] != matchesPerWeek {
			t.Errorf("week %d: got %d matches want %d", w, weekCounts[w], matchesPerWeek)
		}
	}

	// Every ordered pair (home, away) appears exactly once.
	type pair struct{ h, a int64 }
	seen := map[pair]int{}
	for _, m := range matches {
		seen[pair{m.HomeTeamID, m.AwayTeamID}]++
	}
	if len(seen) != TotalMatchesForTeamCount(len(teams)) {
		t.Fatalf("distinct ordered pairs: got %d want %d", len(seen), TotalMatchesForTeamCount(len(teams)))
	}
	for p, n := range seen {
		if n != 1 {
			t.Errorf("pair %+v scheduled %d times, want 1", p, n)
		}
	}

	// Each team plays once per week.
	for w := 1; w <= totalWeeks; w++ {
		teamSeen := map[int64]int{}
		for _, m := range matches {
			if m.Week != w {
				continue
			}
			teamSeen[m.HomeTeamID]++
			teamSeen[m.AwayTeamID]++
		}
		for tid, c := range teamSeen {
			if c != 1 {
				t.Errorf("team %d appears %d times in week %d, want 1", tid, c, w)
			}
		}
	}
}

func TestFixtureGenerator_OddTeamCountHasByes(t *testing.T) {
	teamCount := 5
	rounds := firstLegRounds(teamCount)

	if len(rounds) != teamCount {
		t.Fatalf("rounds=%d want %d", len(rounds), teamCount)
	}

	byes := map[int]int{}
	for week, round := range rounds {
		if len(round) != MatchesPerWeek(teamCount) {
			t.Fatalf("week %d: got %d matches want %d", week+1, len(round), MatchesPerWeek(teamCount))
		}
		seen := map[int]bool{}
		for _, pair := range round {
			seen[pair.home] = true
			seen[pair.away] = true
		}
		if len(seen) != teamCount-1 {
			t.Fatalf("week %d: got %d active teams want %d", week+1, len(seen), teamCount-1)
		}
		for team := 0; team < teamCount; team++ {
			if !seen[team] {
				byes[team]++
			}
		}
	}

	for team := 0; team < teamCount; team++ {
		if byes[team] != 1 {
			t.Errorf("team %d first-leg byes=%d want 1", team, byes[team])
		}
	}
}

func TestFixtureGenerator_WrongTeamCountReturnsNil(t *testing.T) {
	cases := [][]Team{
		nil,
		{},
		{{ID: 1, Name: "A"}},
		append(fourTeams(), Team{ID: 5, Name: "Echo"}),
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("n=%d", len(tc)), func(t *testing.T) {
			_ = i
			if got := (FixtureGenerator{}).Generate(1, tc); got != nil {
				t.Fatalf("expected nil, got %d matches", len(got))
			}
		})
	}
}
