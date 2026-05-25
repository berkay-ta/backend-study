// Package validating wraps any Predictor and enforces the LSP contract:
// entries cover every team once, percentages are in [0,100] and sum to
// 100±tolerance, and expected points are non-negative.
package validating

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

const DefaultTolerance = 0.5

// epsilon bounds the rounding slack in the per-entry table identities.
const epsilon = 0.1

// Retryable is an optional hint implemented by predictors that can re-run
// with feedback (e.g. the AI analyst re-prompting the model).
type Retryable interface {
	PredictWithFeedback(ctx context.Context, in app.PredictionInput, feedback string) (domain.PredictionResult, error)
}

type Predictor struct {
	inner     app.Predictor
	tolerance float64
}

func New(inner app.Predictor) *Predictor {
	return &Predictor{inner: inner, tolerance: DefaultTolerance}
}

func (p *Predictor) WithTolerance(t float64) *Predictor {
	p.tolerance = t
	return p
}

func (p *Predictor) Strategy() domain.Strategy { return p.inner.Strategy() }

func (p *Predictor) Predict(ctx context.Context, in app.PredictionInput) (domain.PredictionResult, error) {
	res, err := p.inner.Predict(ctx, in)
	if err != nil {
		return domain.PredictionResult{}, err
	}
	if verr := Validate(res, in.Teams, p.tolerance); verr != nil {
		retryable, ok := p.inner.(Retryable)
		if !ok {
			return domain.PredictionResult{}, apperror.Internal(
				fmt.Errorf("predictor produced invalid result: %w", verr),
			)
		}
		slog.WarnContext(ctx, "predictor_validation_retry",
			slog.String("strategy", string(p.inner.Strategy())),
			slog.String("reason", verr.Error()),
		)
		res, err = retryable.PredictWithFeedback(ctx, in, verr.Error())
		if err != nil {
			return domain.PredictionResult{}, err
		}
		if verr2 := Validate(res, in.Teams, p.tolerance); verr2 != nil {
			return domain.PredictionResult{}, apperror.BadGateway(apperror.CodeAIPredictionInvalid,
				"AI predictor returned an invalid result after one retry").
				WithCause(verr2)
		}
	}
	return res, nil
}

// Validate enforces the LSP contract; exported so tests can reuse it.
func Validate(res domain.PredictionResult, teams []domain.Team, tolerance float64) error {
	if len(res.Entries) != len(teams) {
		return fmt.Errorf("expected %d entries, got %d", len(teams), len(res.Entries))
	}
	want := make(map[int64]bool, len(teams))
	for _, t := range teams {
		want[t.ID] = true
	}
	seen := make(map[int64]bool, len(teams))
	seenPosition := make(map[int]bool, len(teams))
	var sum float64
	for _, e := range res.Entries {
		if !want[e.TeamID] {
			return fmt.Errorf("entry references unknown team_id=%d", e.TeamID)
		}
		if seen[e.TeamID] {
			return fmt.Errorf("entry team_id=%d appears more than once", e.TeamID)
		}
		seen[e.TeamID] = true
		if e.ChampionshipPct < 0 || e.ChampionshipPct > 100 {
			return fmt.Errorf("team_id=%d championship_pct=%.2f out of [0,100]", e.TeamID, e.ChampionshipPct)
		}
		if e.ProjectedPosition < 1 || e.ProjectedPosition > len(teams) {
			return fmt.Errorf("team_id=%d projected_position=%d out of [1,%d]",
				e.TeamID, e.ProjectedPosition, len(teams))
		}
		if seenPosition[e.ProjectedPosition] {
			return fmt.Errorf("projected_position=%d appears more than once", e.ProjectedPosition)
		}
		seenPosition[e.ProjectedPosition] = true
		if e.AveragePosition < 1 || e.AveragePosition > float64(len(teams)) {
			return fmt.Errorf("team_id=%d average_position=%.2f out of [1,%d]",
				e.TeamID, e.AveragePosition, len(teams))
		}
		if e.Played < 0 {
			return fmt.Errorf("team_id=%d played=%d is negative", e.TeamID, e.Played)
		}
		if e.ExpectedWon < 0 || e.ExpectedDrawn < 0 || e.ExpectedLost < 0 {
			return fmt.Errorf("team_id=%d expected W/D/L contains negative values", e.TeamID)
		}
		if e.ExpectedGoalsFor < 0 || e.ExpectedGoalsAgainst < 0 {
			return fmt.Errorf("team_id=%d expected goals contains negative values", e.TeamID)
		}
		if e.ExpectedPoints < 0 {
			return fmt.Errorf("team_id=%d expected_points=%.2f is negative", e.TeamID, e.ExpectedPoints)
		}
		// Table arithmetic must be consistent: games add up and points follow
		// the 3-1-0 scheme. Catches AI rows whose fields contradict each other.
		if games := e.ExpectedWon + e.ExpectedDrawn + e.ExpectedLost; math.Abs(games-float64(e.Played)) > epsilon {
			return fmt.Errorf("team_id=%d W+D+L=%.2f does not match played=%d", e.TeamID, games, e.Played)
		}
		if pts := 3*e.ExpectedWon + e.ExpectedDrawn; math.Abs(pts-e.ExpectedPoints) > epsilon {
			return fmt.Errorf("team_id=%d 3*W+D=%.2f does not match expected_points=%.2f", e.TeamID, pts, e.ExpectedPoints)
		}
		sum += e.ChampionshipPct
	}
	if math.Abs(sum-100) > tolerance {
		return fmt.Errorf("championship_pct sum=%.2f, expected 100±%.2f", sum, tolerance)
	}
	return nil
}
