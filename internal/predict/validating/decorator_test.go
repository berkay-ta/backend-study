package validating_test

import (
	"context"
	"errors"
	"testing"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
	"github.com/iberkayC/case1back/internal/predict/validating"
)

func teams() []domain.Team {
	return []domain.Team{
		{ID: 1, Name: "A"}, {ID: 2, Name: "B"},
		{ID: 3, Name: "C"}, {ID: 4, Name: "D"},
	}
}

func validResult() domain.PredictionResult {
	return domain.PredictionResult{
		Strategy: domain.StrategyMonteCarlo,
		Entries: []domain.PredictionEntry{
			predEntry(1, 1, 40, 14),
			predEntry(2, 2, 30, 12),
			predEntry(3, 3, 20, 9),
			predEntry(4, 4, 10, 6),
		},
	}
}

func predEntry(teamID int64, pos int, pct, points float64) domain.PredictionEntry {
	return domain.PredictionEntry{
		ProjectedPosition:    pos,
		TeamID:               teamID,
		ChampionshipPct:      pct,
		AveragePosition:      float64(pos),
		Played:               6,
		ExpectedWon:          points / 3,
		ExpectedDrawn:        0,
		ExpectedLost:         6 - points/3,
		ExpectedGoalsFor:     points,
		ExpectedGoalsAgainst: 6,
		ExpectedGoalDiff:     points - 6,
		ExpectedPoints:       points,
	}
}

// fakePredictor lets tests script up to two predictions on the inner side.
type fakePredictor struct {
	strategy domain.Strategy
	calls    int
	results  []domain.PredictionResult
	errs     []error
}

func (f *fakePredictor) Strategy() domain.Strategy { return f.strategy }
func (f *fakePredictor) Predict(_ context.Context, _ app.PredictionInput) (domain.PredictionResult, error) {
	defer func() { f.calls++ }()
	if f.calls < len(f.errs) && f.errs[f.calls] != nil {
		return domain.PredictionResult{}, f.errs[f.calls]
	}
	return f.results[f.calls], nil
}

// retryableFake also implements validating.Retryable.
type retryableFake struct {
	*fakePredictor
	feedbackSeen string
}

func (r *retryableFake) PredictWithFeedback(_ context.Context, _ app.PredictionInput, feedback string) (domain.PredictionResult, error) {
	r.feedbackSeen = feedback
	return r.Predict(context.Background(), app.PredictionInput{})
}

func TestValidator_PassesThroughValidResult(t *testing.T) {
	t.Parallel()

	inner := &fakePredictor{strategy: domain.StrategyMonteCarlo, results: []domain.PredictionResult{validResult()}}
	p := validating.New(inner)

	res, err := p.Predict(context.Background(), app.PredictionInput{Teams: teams()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(res.Entries))
	}
	if inner.calls != 1 {
		t.Errorf("expected 1 inner call, got %d", inner.calls)
	}
}

func TestValidator_NonRetryable_BadResult_ReturnsInternal(t *testing.T) {
	t.Parallel()

	bad := validResult()
	bad.Entries[0].ChampionshipPct = 200 // out of range
	inner := &fakePredictor{strategy: domain.StrategyMonteCarlo, results: []domain.PredictionResult{bad}}
	p := validating.New(inner)

	_, err := p.Predict(context.Background(), app.PredictionInput{Teams: teams()})
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("expected *apperror.Error, got %T", err)
	}
	if ae.Code != apperror.CodeInternal {
		t.Errorf("expected CodeInternal, got %s", ae.Code)
	}
}

func TestValidator_Retryable_RecoversOnRetry(t *testing.T) {
	t.Parallel()

	bad := validResult()
	bad.Entries[0].ChampionshipPct = 0
	bad.Entries[1].ChampionshipPct = 0 // sum != 100

	inner := &retryableFake{fakePredictor: &fakePredictor{
		strategy: domain.StrategyAIAnalyst,
		results:  []domain.PredictionResult{bad, validResult()},
	}}
	p := validating.New(inner)

	res, err := p.Predict(context.Background(), app.PredictionInput{Teams: teams()})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if inner.calls != 2 {
		t.Errorf("expected 2 inner calls, got %d", inner.calls)
	}
	if inner.feedbackSeen == "" {
		t.Error("expected feedback to be passed on retry")
	}
	if len(res.Entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(res.Entries))
	}
}

func TestValidator_Retryable_StillBad_ReturnsAIPredictionInvalid(t *testing.T) {
	t.Parallel()

	bad := validResult()
	bad.Entries[3].ChampionshipPct = -5
	inner := &retryableFake{fakePredictor: &fakePredictor{
		strategy: domain.StrategyAIAnalyst,
		results:  []domain.PredictionResult{bad, bad},
	}}
	p := validating.New(inner)

	_, err := p.Predict(context.Background(), app.PredictionInput{Teams: teams()})
	if err == nil {
		t.Fatal("expected error after second invalid attempt")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("expected *apperror.Error, got %T", err)
	}
	if ae.Status != 502 || ae.Code != apperror.CodeAIPredictionInvalid {
		t.Errorf("expected 502 ai_prediction_invalid, got %d %s", ae.Status, ae.Code)
	}
}

func TestValidate_DetectsMissingTeam(t *testing.T) {
	t.Parallel()

	r := validResult()
	r.Entries = r.Entries[:3] // drop one
	err := validating.Validate(r, teams(), 0.5)
	if err == nil {
		t.Fatal("expected validation error for missing team")
	}
}

func TestValidator_PropagatesInnerError(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	inner := &fakePredictor{
		strategy: domain.StrategyMonteCarlo,
		results:  []domain.PredictionResult{{}},
		errs:     []error{want},
	}
	p := validating.New(inner)

	_, err := p.Predict(context.Background(), app.PredictionInput{Teams: teams()})
	if !errors.Is(err, want) {
		t.Errorf("expected inner error to propagate, got %v", err)
	}
}
