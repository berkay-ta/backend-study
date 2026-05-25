package domain

// WhatIfApply returns a new match slice with the overrides applied by ID.
// Unknown match IDs are ignored. The original slice is not mutated.
func WhatIfApply(matches []Match, overrides []MatchOverride) []Match {
	out := make([]Match, len(matches))
	copy(out, matches)
	if len(overrides) == 0 {
		return out
	}
	byID := make(map[int64]MatchOverride, len(overrides))
	for _, o := range overrides {
		byID[o.MatchID] = o
	}
	for i, m := range out {
		o, ok := byID[m.ID]
		if !ok {
			continue
		}
		hg, ag := o.HomeGoals, o.AwayGoals
		m.HomeGoals = &hg
		m.AwayGoals = &ag
		out[i] = m
	}
	return out
}
