// Package montecarlo implements a deterministic championship predictor.
package montecarlo

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/fnv"
	"math"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/platform/apperror"
	"github.com/iberkayC/case1back/internal/platform/random"
)

const ctxCheckInterval = 256

type Predictor struct {
	simulations int
	calc        domain.StandingCalculator
	gen         domain.ResultGenerator
}

func New(simulations int) *Predictor {
	if simulations < 1 {
		simulations = 1
	}
	return &Predictor{simulations: simulations}
}

func (p *Predictor) Strategy() domain.Strategy { return domain.StrategyMonteCarlo }

func (p *Predictor) Predict(ctx context.Context, in app.PredictionInput) (domain.PredictionResult, error) {
	if len(in.Teams) == 0 {
		return domain.PredictionResult{}, apperror.Internal(errors.New("montecarlo: no teams"))
	}
	if err := ctx.Err(); err != nil {
		return domain.PredictionResult{}, err
	}

	teamByID := domain.TeamsByID(in.Teams)
	championships := make(map[int64]float64, len(in.Teams))
	projected := make(map[int64]rowTotals, len(in.Teams))

	rng := random.NewSeeded(seedFor(in.League.ID, in.League.Version, p.simulations))

	for trial := 0; trial < p.simulations; trial++ {
		if trial%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return domain.PredictionResult{}, err
			}
		}

		simulated := make([]domain.Match, 0, len(in.PlayedMatches)+len(in.Remaining))
		simulated = append(simulated, in.PlayedMatches...)
		for _, m := range in.Remaining {
			home, okH := teamByID[m.HomeTeamID]
			away, okA := teamByID[m.AwayTeamID]
			if !okH || !okA {
				return domain.PredictionResult{}, apperror.Internal(
					fmt.Errorf("montecarlo: team not found for match %d", m.ID),
				)
			}
			hg, ag := p.gen.Play(home, away, rng)
			h, a := hg, ag
			m.HomeGoals = &h
			m.AwayGoals = &a
			simulated = append(simulated, m)
		}

		rows := p.calc.Compute(in.Teams, simulated)
		if len(rows) == 0 {
			continue
		}
		for teamID, share := range domain.ChampionShares(rows) {
			championships[teamID] += share
		}
		for _, r := range rows {
			total := projected[r.TeamID]
			total.Position += float64(r.Position)
			total.Played += float64(r.Played)
			total.Won += float64(r.Won)
			total.Drawn += float64(r.Drawn)
			total.Lost += float64(r.Lost)
			total.GoalsFor += float64(r.GoalsFor)
			total.GoalsAgainst += float64(r.GoalsAgainst)
			total.GoalDiff += float64(r.GoalDiff)
			total.Points += float64(r.Points)
			projected[r.TeamID] = total
		}
	}

	trials := float64(p.simulations)
	entries := make([]domain.PredictionEntry, 0, len(in.Teams))
	for _, t := range in.Teams {
		total := projected[t.ID]
		entries = append(entries, domain.PredictionEntry{
			TeamID:               t.ID,
			TeamName:             t.Name,
			ChampionshipPct:      round2(championships[t.ID] / trials * 100),
			AveragePosition:      round2(total.Position / trials),
			Played:               int(math.Round(total.Played / trials)),
			ExpectedWon:          round2(total.Won / trials),
			ExpectedDrawn:        round2(total.Drawn / trials),
			ExpectedLost:         round2(total.Lost / trials),
			ExpectedGoalsFor:     round2(total.GoalsFor / trials),
			ExpectedGoalsAgainst: round2(total.GoalsAgainst / trials),
			ExpectedGoalDiff:     round2(total.GoalDiff / trials),
			ExpectedPoints:       round2(total.Points / trials),
		})
	}
	normaliseTo100(entries)
	domain.SortPredictionEntries(entries)

	return domain.PredictionResult{
		Strategy: domain.StrategyMonteCarlo,
		Entries:  entries,
	}, nil
}

type rowTotals struct {
	Position     float64
	Played       float64
	Won          float64
	Drawn        float64
	Lost         float64
	GoalsFor     float64
	GoalsAgainst float64
	GoalDiff     float64
	Points       float64
}

// normaliseTo100 nudges the largest entry so percentages sum to exactly 100.
func normaliseTo100(entries []domain.PredictionEntry) {
	if len(entries) == 0 {
		return
	}
	sum := 0.0
	for _, e := range entries {
		sum += e.ChampionshipPct
	}
	diff := 100.0 - sum
	if diff == 0 {
		return
	}
	idx := 0
	for i, e := range entries {
		if e.ChampionshipPct > entries[idx].ChampionshipPct {
			idx = i
		}
	}
	entries[idx].ChampionshipPct = round2(entries[idx].ChampionshipPct + diff)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func seedFor(leagueID, version int64, sims int) uint64 {
	h := fnv.New64a()
	_ = binary.Write(h, binary.LittleEndian, uint64(leagueID))
	_ = binary.Write(h, binary.LittleEndian, uint64(version))
	_ = binary.Write(h, binary.LittleEndian, uint64(sims))
	return h.Sum64()
}
