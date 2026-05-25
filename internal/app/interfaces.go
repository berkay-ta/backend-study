package app

import (
	"context"

	"github.com/iberkayC/case1back/internal/domain"
)

type LeagueReader interface {
	List(ctx context.Context) ([]domain.League, error)
	Get(ctx context.Context, id int64) (domain.League, error)
	GetTeams(ctx context.Context, leagueID int64) ([]domain.Team, error)
}

type LeagueWriter interface {
	Create(ctx context.Context, name string, totalWeeks int, teams []domain.Team) (domain.League, []domain.Team, error)
	Delete(ctx context.Context, id int64) error
	GetForUpdate(ctx context.Context, id int64) (domain.League, error)
	BumpVersion(ctx context.Context, id int64) (int64, error)
	SetCurrentWeek(ctx context.Context, id int64, week int) error
}

type LeagueRepository interface {
	LeagueReader
	LeagueWriter
}

type MatchReader interface {
	List(ctx context.Context, leagueID int64) ([]domain.Match, error)
	ListByWeek(ctx context.Context, leagueID int64, week int) ([]domain.Match, error)
	Get(ctx context.Context, matchID int64) (domain.Match, error)
}

type MatchWriter interface {
	InsertFixtures(ctx context.Context, matches []domain.Match) ([]domain.Match, error)
	RecordResult(ctx context.Context, matchID int64, homeGoals, awayGoals int, edited bool) error
	ClearAllResults(ctx context.Context, leagueID int64) error
}

type MatchRepository interface {
	MatchReader
	MatchWriter
}

type StandingSnapshotRepository interface {
	GetOrCreate(ctx context.Context, snap domain.StandingSnapshot) (domain.StandingSnapshot, error)
	Get(ctx context.Context, snapshotID int64) (domain.StandingSnapshot, error)
}

type PredictionRepository interface {
	Create(ctx context.Context, run domain.PredictionRun) (domain.PredictionRun, error)
	Get(ctx context.Context, runID int64) (domain.PredictionRun, error)
	ListByLeague(ctx context.Context, leagueID int64) ([]domain.PredictionRun, error)
	MarkStaleBefore(ctx context.Context, leagueID int64, leagueVersion int64) error
}

type IdempotencyState string

const (
	IdempotencyInProgress IdempotencyState = "in_progress"
	IdempotencyCompleted  IdempotencyState = "completed"
)

type IdempotencyRecord struct {
	Key            string
	RequestHash    string
	State          IdempotencyState
	ResponseStatus int
	ResponseBody   []byte
}

type IdempotencyRepository interface {
	Reserve(ctx context.Context, key string, requestHash string) (IdempotencyRecord, bool, error)
	Complete(ctx context.Context, key string, status int, body []byte) error
	Abort(ctx context.Context, key string) error
}

// TxRunner runs fn inside a database transaction. Repositories detect the
// bound *sql.Tx via context.Value.
type TxRunner interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Predictor interface {
	Strategy() domain.Strategy
	Predict(ctx context.Context, input PredictionInput) (domain.PredictionResult, error)
}

type PredictionInput struct {
	League        domain.League
	Teams         []domain.Team
	PlayedMatches []domain.Match
	Remaining     []domain.Match
	Snapshot      []domain.StandingRow
}
