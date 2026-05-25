package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

// PredictionService runs predictors against the current league state and
// persists the runs.
type PredictionService struct {
	leagues    LeagueRepository
	matches    MatchRepository
	snapshots  StandingSnapshotRepository
	runs       PredictionRepository
	tx         TxRunner
	calc       domain.StandingCalculator
	predictors map[domain.Strategy]Predictor
}

// NewPredictionService bundles predictors by their declared Strategy. An
// empty list is fine (e.g. AI disabled); Create returns ErrUnknownStrategy.
func NewPredictionService(
	leagues LeagueRepository,
	matches MatchRepository,
	snapshots StandingSnapshotRepository,
	runs PredictionRepository,
	tx TxRunner,
	predictors []Predictor,
) *PredictionService {
	m := make(map[domain.Strategy]Predictor, len(predictors))
	for _, p := range predictors {
		m[p.Strategy()] = p
	}
	return &PredictionService{
		leagues: leagues, matches: matches, snapshots: snapshots,
		runs: runs, tx: tx, predictors: m,
	}
}

// MarkPriorRunsStale is the mutation hook; callers invoke it inside their tx so
// it commits atomically with the version bump.
func (s *PredictionService) MarkPriorRunsStale(ctx context.Context, leagueID, newVersion int64) error {
	return s.runs.MarkStaleBefore(ctx, leagueID, newVersion)
}

func (s *PredictionService) Create(ctx context.Context, leagueID int64, strategy domain.Strategy) (domain.PredictionRun, error) {
	predictor, err := s.lookupPredictor(strategy)
	if err != nil {
		return domain.PredictionRun{}, err
	}

	// Phase 1: capture the input lock-free. The single REPEATABLE READ tx gives
	// all three reads one snapshot, so they can't tear against a mutation;
	// baseVersion is re-checked under the lock in Phase 3.
	var (
		input       PredictionInput
		baseVersion int64
		snapshotWk  int
	)
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		league, err := s.leagues.Get(ctx, leagueID)
		if err != nil {
			return err
		}
		if err := predictionAvailabilityError(league); err != nil {
			return err
		}
		in, err := s.buildInput(ctx, league)
		if err != nil {
			return err
		}
		input = in
		baseVersion = league.Version
		snapshotWk = league.CurrentWeek - 1
		return nil
	})
	if err != nil {
		return domain.PredictionRun{}, err
	}

	// Phase 2: simulate with no lock held, against the frozen input.
	result, err := predictor.Predict(ctx, input)
	if err != nil {
		return domain.PredictionRun{}, err
	}

	// Phase 3: persist snapshot and run under the lock, re-checking version. A
	// mutation during simulation makes baseVersion stale, so the run is born
	// stale; later mutations flag it via MarkStaleBefore.
	var saved domain.PredictionRun
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		current, err := s.leagues.GetForUpdate(ctx, leagueID)
		if err != nil {
			return err
		}
		stale := current.Version != baseVersion
		snap, err := s.snapshots.GetOrCreate(ctx, domain.StandingSnapshot{
			LeagueID:      leagueID,
			SnapshotWeek:  snapshotWk,
			LeagueVersion: baseVersion,
			Rows:          input.Snapshot,
		})
		if err != nil {
			return err
		}
		saved, err = s.runs.Create(ctx, domain.PredictionRun{
			LeagueID:      leagueID,
			Strategy:      strategy,
			SnapshotID:    snap.ID,
			LeagueVersion: baseVersion,
			Stale:         stale,
			Result:        result,
		})
		return err
	})
	if err != nil {
		return domain.PredictionRun{}, err
	}
	slog.InfoContext(ctx, "prediction_generated",
		slog.Int64("league_id", saved.LeagueID),
		slog.Int64("run_id", saved.ID),
		slog.String("strategy", string(saved.Strategy)),
		slog.Int64("league_version", saved.LeagueVersion),
		slog.Bool("stale", saved.Stale),
		slog.Int("entries", len(saved.Result.Entries)),
	)
	return saved, nil
}

func (s *PredictionService) Get(ctx context.Context, runID int64) (domain.PredictionRun, error) {
	return s.runs.Get(ctx, runID)
}

func (s *PredictionService) GetForLeague(ctx context.Context, leagueID, runID int64) (domain.PredictionRun, error) {
	run, err := s.runs.Get(ctx, runID)
	if err != nil {
		return domain.PredictionRun{}, err
	}
	if run.LeagueID != leagueID {
		return domain.PredictionRun{}, domain.ErrPredictionNotFound
	}
	return run, nil
}

func (s *PredictionService) ListByLeague(ctx context.Context, leagueID int64) ([]domain.PredictionRun, error) {
	if _, err := s.leagues.Get(ctx, leagueID); err != nil {
		return nil, err
	}
	return s.runs.ListByLeague(ctx, leagueID)
}

type WhatIfInput struct {
	LeagueID  int64
	Overrides []domain.MatchOverride
	Strategy  domain.Strategy // optional; empty means standings only
}

type WhatIfResult struct {
	Standings  []domain.StandingRow
	Prediction *domain.PredictionResult
}

func (s *PredictionService) WhatIf(ctx context.Context, in WhatIfInput) (WhatIfResult, error) {
	league, err := s.leagues.Get(ctx, in.LeagueID)
	if err != nil {
		return WhatIfResult{}, err
	}
	teams, err := s.leagues.GetTeams(ctx, in.LeagueID)
	if err != nil {
		return WhatIfResult{}, err
	}
	matches, err := s.matches.List(ctx, in.LeagueID)
	if err != nil {
		return WhatIfResult{}, err
	}
	if err := validateOverrides(in.Overrides, matches); err != nil {
		return WhatIfResult{}, err
	}

	synthetic := domain.WhatIfApply(matches, in.Overrides)
	out := WhatIfResult{Standings: s.calc.Compute(teams, synthetic)}

	if in.Strategy == "" {
		return out, nil
	}
	predictor, err := s.lookupPredictor(in.Strategy)
	if err != nil {
		return WhatIfResult{}, err
	}
	if err := predictionAvailabilityError(league); err != nil && !canBypassPredictionMinWeek(err, synthetic) {
		return WhatIfResult{}, err
	}

	played, remaining := splitPlayedRemaining(synthetic)
	result, err := predictor.Predict(ctx, PredictionInput{
		League:        league,
		Teams:         teams,
		PlayedMatches: played,
		Remaining:     remaining,
		Snapshot:      out.Standings,
	})
	if err != nil {
		return WhatIfResult{}, err
	}
	out.Prediction = &result
	return out, nil
}

func predictionAvailabilityError(league domain.League) error {
	if league.IsComplete() {
		return domain.ErrLeagueAlreadyComplete
	}
	if !league.PredictionsOpen() {
		return domain.ErrPredictionNotAvailable
	}
	return nil
}

func canBypassPredictionMinWeek(err error, matches []domain.Match) bool {
	return err == domain.ErrPredictionNotAvailable && overridesCoverEnoughWeeks(matches)
}

func (s *PredictionService) lookupPredictor(strategy domain.Strategy) (Predictor, error) {
	if !domain.KnownStrategy(strategy) {
		return nil, domain.ErrUnknownStrategy
	}
	p, ok := s.predictors[strategy]
	if ok {
		return p, nil
	}
	if strategy == domain.StrategyAIAnalyst {
		return nil, domain.ErrAIDisabled
	}
	return nil, domain.ErrUnknownStrategy
}

// buildInput assembles the prediction input for an already-loaded league. It
// runs inside the phase-1 transaction, so its team and match reads share one
// consistent snapshot.
func (s *PredictionService) buildInput(ctx context.Context, league domain.League) (PredictionInput, error) {
	teams, err := s.leagues.GetTeams(ctx, league.ID)
	if err != nil {
		return PredictionInput{}, err
	}
	matches, err := s.matches.List(ctx, league.ID)
	if err != nil {
		return PredictionInput{}, err
	}
	played, remaining := splitPlayedRemaining(matches)
	return PredictionInput{
		League:        league,
		Teams:         teams,
		PlayedMatches: played,
		Remaining:     remaining,
		Snapshot:      s.calc.Compute(teams, played),
	}, nil
}

func splitPlayedRemaining(matches []domain.Match) (played, remaining []domain.Match) {
	for _, m := range matches {
		if m.Played() {
			played = append(played, m)
		} else {
			remaining = append(remaining, m)
		}
	}
	return
}

// overridesCoverEnoughWeeks lets what-if bypass the week-4 gate when the
// synthetic schedule has complete results through the prediction opening week.
func overridesCoverEnoughWeeks(matches []domain.Match) bool {
	type counts struct{ played, total int }
	byWeek := map[int]counts{}
	for _, m := range matches {
		c := byWeek[m.Week]
		c.total++
		if m.Played() {
			c.played++
		}
		byWeek[m.Week] = c
	}
	completeWeeks := 0
	for week := 1; ; week++ {
		c, ok := byWeek[week]
		if !ok || c.played != c.total {
			break
		}
		completeWeeks++
	}
	return completeWeeks >= domain.PredictionMinWeek
}

func validateOverrides(overrides []domain.MatchOverride, matches []domain.Match) error {
	if len(overrides) == 0 {
		return nil
	}
	matchSet := make(map[int64]struct{}, len(matches))
	for _, m := range matches {
		matchSet[m.ID] = struct{}{}
	}
	var fields []apperror.FieldError
	seen := map[int64]bool{}
	for i, o := range overrides {
		if _, ok := matchSet[o.MatchID]; !ok {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("overrides[%d].match_id", i),
				Message: "no such match in this league",
			})
		}
		if seen[o.MatchID] {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("overrides[%d].match_id", i),
				Message: "duplicate override for same match",
			})
		}
		seen[o.MatchID] = true
		if o.HomeGoals < 0 || o.AwayGoals < 0 {
			fields = append(fields, apperror.FieldError{
				Field:   fmt.Sprintf("overrides[%d]", i),
				Message: "goals must be non-negative",
			})
		}
	}
	if len(fields) > 0 {
		return apperror.ValidationFailed(fields...)
	}
	return nil
}
