package domain

import (
	"testing"

	"github.com/iberkayC/case1back/internal/platform/random"
)

func TestResultGenerator_DeterministicWithSeededRNG(t *testing.T) {
	home := Team{ID: 1, Name: "Alpha", Strength: 85}
	away := Team{ID: 2, Name: "Bravo", Strength: 60}

	r1 := random.NewSeeded(11)
	r2 := random.NewSeeded(11)
	rg := ResultGenerator{}

	for i := 0; i < 50; i++ {
		h1, a1 := rg.Play(home, away, r1)
		h2, a2 := rg.Play(home, away, r2)
		if h1 != h2 || a1 != a2 {
			t.Fatalf("iter %d: divergence between equally-seeded runs: %d-%d vs %d-%d",
				i, h1, a1, h2, a2)
		}
	}
}

func TestResultGenerator_GoalsBoundedAndNonNegative(t *testing.T) {
	home := Team{ID: 1, Name: "A", Strength: 99}
	away := Team{ID: 2, Name: "B", Strength: 1}
	rg := ResultGenerator{}
	rng := random.NewSeeded(11)

	for i := 0; i < 1000; i++ {
		h, a := rg.Play(home, away, rng)
		if h < 0 || a < 0 {
			t.Fatalf("iter %d: negative goals %d-%d", i, h, a)
		}
		if h > 12 || a > 12 {
			t.Fatalf("iter %d: goals above bound %d-%d", i, h, a)
		}
	}
}
