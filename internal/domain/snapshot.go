package domain

import "time"

// StandingSnapshot is an immutable league table at a (week, version). Used as
// the prediction input so historic runs remain meaningful after edits.
type StandingSnapshot struct {
	ID            int64
	LeagueID      int64
	SnapshotWeek  int
	LeagueVersion int64
	CreatedAt     time.Time
	Rows          []StandingRow
}
