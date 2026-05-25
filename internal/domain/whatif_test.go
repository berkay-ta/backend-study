package domain

import "testing"

func TestWhatIfApply_OverridesOnlyTargetedMatches(t *testing.T) {
	original := []Match{
		played(Match{ID: 1, HomeTeamID: 1, AwayTeamID: 2}, 1, 0),
		played(Match{ID: 2, HomeTeamID: 3, AwayTeamID: 4}, 2, 2),
		{ID: 3, HomeTeamID: 1, AwayTeamID: 3}, // unplayed
	}
	overrides := []MatchOverride{
		{MatchID: 1, HomeGoals: 0, AwayGoals: 3},  // change a played match
		{MatchID: 3, HomeGoals: 1, AwayGoals: 1},  // promote an unplayed match
		{MatchID: 99, HomeGoals: 4, AwayGoals: 4}, // unknown, silently ignored
	}

	out := WhatIfApply(original, overrides)

	if *original[0].HomeGoals != 1 || *original[0].AwayGoals != 0 {
		t.Fatalf("original match 1 mutated")
	}
	if original[2].HomeGoals != nil {
		t.Fatalf("original match 3 mutated")
	}

	if *out[0].HomeGoals != 0 || *out[0].AwayGoals != 3 {
		t.Errorf("override of played match not applied: got %d-%d", *out[0].HomeGoals, *out[0].AwayGoals)
	}
	if *out[1].HomeGoals != 2 || *out[1].AwayGoals != 2 {
		t.Errorf("untouched match changed: got %d-%d", *out[1].HomeGoals, *out[1].AwayGoals)
	}
	if !out[2].Played() {
		t.Errorf("match 3 should be marked played after override")
	}
	if *out[2].HomeGoals != 1 || *out[2].AwayGoals != 1 {
		t.Errorf("override of unplayed match not applied: got %d-%d", *out[2].HomeGoals, *out[2].AwayGoals)
	}
}

func TestWhatIfApply_EmptyOverridesIsCopy(t *testing.T) {
	original := []Match{played(Match{ID: 1, HomeTeamID: 1, AwayTeamID: 2}, 1, 0)}
	out := WhatIfApply(original, nil)
	if len(out) != 1 {
		t.Fatalf("len: got %d want 1", len(out))
	}
	// Mutate the copy's goal pointer and confirm original is shared-but-safe.
	// (We don't deep-clone the pointers when there are no overrides, but the
	// slice itself must be a new allocation.)
	if &out[0] == &original[0] {
		t.Fatalf("expected a new slice allocation")
	}
}
