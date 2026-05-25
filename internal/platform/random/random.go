// Package random provides a seedable randomness seam.
package random

import (
	"encoding/binary"
	"math/rand/v2"
	"sync"
)

type Randomizer interface {
	IntN(n int) int
	Float64() float64
}

// NewSeeded returns a deterministic Randomizer seeded with the given value.
func NewSeeded(seed uint64) Randomizer {
	return &chacha8{r: rand.New(rand.NewChaCha8(toBytes(seed)))}
}

// NewDefault returns a non-deterministic Randomizer.
func NewDefault() Randomizer {
	return &globalRand{}
}

type chacha8 struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (c *chacha8) IntN(n int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.r.IntN(n)
}

func (c *chacha8) Float64() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.r.Float64()
}

type globalRand struct{}

func (globalRand) IntN(n int) int   { return rand.IntN(n) }
func (globalRand) Float64() float64 { return rand.Float64() }

func toBytes(seed uint64) [32]byte {
	var b [32]byte
	binary.LittleEndian.PutUint64(b[:8], seed)
	return b
}
