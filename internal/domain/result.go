package domain

import (
	"math"
)

// Randomizer is the minimal random source needed by domain scoring logic.
type Randomizer interface {
	Float64() float64
}

// HomeAdvantage scales the home team's expected goal rate. 1.15 ≈ +15%.
const HomeAdvantage = 1.15

// ResultGenerator scores a match by drawing each side's goals from a Poisson
// process whose rate is derived from the strength delta.
type ResultGenerator struct{}

// Play returns (homeGoals, awayGoals). Deterministic for a seeded rng.
func (ResultGenerator) Play(home, away Team, rng Randomizer) (int, int) {
	hLambda := expectedGoals(home.Strength, away.Strength) * HomeAdvantage
	aLambda := expectedGoals(away.Strength, home.Strength)
	return poisson(rng, hLambda), poisson(rng, aLambda)
}

// expectedGoals maps the (attack-defence) gap to a Poisson rate, clamped so a
// 1-vs-100 gap stays plausible.
func expectedGoals(attack, defence int) float64 {
	// 1.3 for equally matched teams
	const base = 1.3
	rate := base + float64(attack-defence)/25.0
	if rate < 0.2 {
		return 0.2
	}
	if rate > 4.5 {
		return 4.5
	}
	return rate
}

// poisson samples a Poisson(\lambda) integer via Knuth's algorithm, capped at 12.
func poisson(rng Randomizer, lambda float64) int {
	L := math.Exp(-lambda)
	k := 0
	p := 1.0
	for {
		k++
		p *= rng.Float64()
		if p <= L {
			return k - 1
		}
		if k > 12 {
			return 12
		}
	}
}
