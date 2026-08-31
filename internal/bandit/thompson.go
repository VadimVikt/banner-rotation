// Package bandit implements Thompson Sampling (Beta-Bernoulli) using only Go stdlib.
package bandit

import (
	"math"
	"math/rand"
	"sync"
)

// arm holds the parameters of a Beta distribution for a single bandit arm.
type arm struct {
	alpha float64 // successes + 1 (prior)
	beta  float64 // failures + 1 (prior)
}

// Bandit implements Thompson Sampling with Beta-Bernoulli model.
// Each arm is identified by a string ID and maintains independent Beta(alpha, beta) parameters.
type Bandit struct {
	mu   sync.Mutex
	rng  *rand.Rand
	arms map[string]*arm
}

// NewBandit creates a new Bandit ready to accept arms.
func NewBandit() *Bandit {
	return &Bandit{
		rng:  rand.New(rand.NewSource(0)), // deterministic seed; caller may change via rand.Seed if needed
		arms: make(map[string]*arm),
	}
}

// AddArm registers a new arm (banner) with uninformative prior Beta(1,1).
func (b *Bandit) AddArm(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.arms[id] = &arm{alpha: 1, beta: 1}
}

// RemoveArm removes an arm by ID. No-op if the arm does not exist.
func (b *Bandit) RemoveArm(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.arms, id)
}

// Update records an observation for the given arm.
// If clicked is true, alpha is incremented; otherwise beta is incremented.
// No-op if the arm does not exist.
func (b *Bandit) Update(id string, clicked bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	a, ok := b.arms[id]
	if !ok {
		return
	}
	if clicked {
		a.alpha++
	} else {
		a.beta++
	}
}

// Pick returns the ID of the arm with the highest Thompson sample.
// Returns "" if no arms are registered.
func (b *Bandit) Pick() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	bestID := ""
	bestSample := -1.0

	for id, a := range b.arms {
		s := betaSample(b.rng, a.alpha, a.beta)
		if s > bestSample {
			bestSample = s
			bestID = id
		}
	}

	return bestID
}

// PickWithImpression picks an arm and records an impression (beta++ for the chosen arm).
// Returns "" if no arms are registered.
func (b *Bandit) PickWithImpression() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	bestID := ""
	bestSample := -1.0

	for id, a := range b.arms {
		s := betaSample(b.rng, a.alpha, a.beta)
		if s > bestSample {
			bestSample = s
			bestID = id
		}
	}

	if bestID != "" {
		b.arms[bestID].beta++
	}

	return bestID
}

// betaSample draws a sample from Beta(alpha, beta) using Marsaglia-Tsang Gamma.
func betaSample(rng *rand.Rand, alpha, beta float64) float64 {
	x := gammaMT(rng, alpha)
	y := gammaMT(rng, beta)
	return x / (x + y)
}

// gammaMT draws a sample from Gamma(shape, 1) using the Marsaglia & Tsang method.
// For shape < 1 it uses the transformation: Gamma(d,1) = Gamma(d+1,1) * U^(1/d).
func gammaMT(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		return gammaMT(rng, shape+1) * math.Pow(rng.Float64(), 1.0/shape)
	}

	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0 * d)

	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1.0 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1.0+0.33756*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1.0-v+math.Log(v)) {
			return d * v
		}
	}
}