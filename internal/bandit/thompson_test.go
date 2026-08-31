package bandit

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

// TestPickReturnsExistingArm verifies that Pick() only returns registered arm IDs.
func TestPickReturnsExistingArm(t *testing.T) {
	b := NewBandit()
	arms := []string{"a", "b", "c", "d", "e"}
	for _, id := range arms {
		b.AddArm(id)
	}

	armSet := make(map[string]bool)
	for _, id := range arms {
		armSet[id] = true
	}

	for i := 0; i < 1000; i++ {
		picked := b.Pick()
		if !armSet[picked] {
			t.Fatalf("iteration %d: Pick() returned unknown arm %q", i, picked)
		}
	}
}

// TestEachArmShownAtLeastOnce verifies that after many picks every arm is chosen at least once.
func TestEachArmShownAtLeastOnce(t *testing.T) {
	b := NewBandit()
	arms := []string{"a", "b", "c", "d", "e"}
	for _, id := range arms {
		b.AddArm(id)
	}

	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		picked := b.Pick()
		counts[picked]++
	}

	for _, id := range arms {
		if counts[id] == 0 {
			t.Errorf("arm %q was never picked in 1000 iterations; counts: %v", id, counts)
		}
	}
}

// TestPopularArmGetsMoreImpressions verifies that a high-CTR arm dominates over time.
func TestPopularArmGetsMoreImpressions(t *testing.T) {
	b := NewBandit()
	b.AddArm("good")
	b.AddArm("ok")
	b.AddArm("bad")

	// Separate RNG for click simulation to avoid data races with b.rng
	simRng := rand.New(rand.NewSource(12345))

	// Learning phase: 1000 iterations
	for i := 0; i < 1000; i++ {
		picked := b.PickWithImpression()

		// Simulate click: "good" clicks 80% of time, others ~10%
		clicked := false
		switch picked {
		case "good":
			clicked = simRng.Float64() < 0.8
		case "ok":
			clicked = simRng.Float64() < 0.1
		case "bad":
			clicked = simRng.Float64() < 0.05
		}
		b.Update(picked, clicked)
	}

	// Evaluation phase: 1000 picks
	counts := make(map[string]int)
	for i := 0; i < 1000; i++ {
		picked := b.Pick()
		counts[picked]++
	}

	// "good" should dominate — at least 50% of picks
	if counts["good"] < 500 {
		t.Errorf("good arm picked only %d/1000 times (expected >= 500); counts: %v", counts["good"], counts)
	}
}

// TestAddRemoveArm verifies dynamic arm registration and removal.
func TestAddRemoveArm(t *testing.T) {
	b := NewBandit()

	// Initially empty
	if got := b.Pick(); got != "" {
		t.Fatalf("expected empty pick, got %q", got)
	}

	b.AddArm("x")
	b.AddArm("y")

	// Should only pick x or y
	for i := 0; i < 100; i++ {
		picked := b.Pick()
		if picked != "x" && picked != "y" {
			t.Fatalf("unexpected pick %q", picked)
		}
	}

	b.RemoveArm("x")

	// Should only pick y now
	for i := 0; i < 100; i++ {
		picked := b.Pick()
		if picked != "y" {
			t.Fatalf("after removing x, expected y, got %q", picked)
		}
	}

	// Remove last arm — should return empty
	b.RemoveArm("y")
	if got := b.Pick(); got != "" {
		t.Fatalf("expected empty pick after removing all arms, got %q", got)
	}
}

// TestEmptyBanditReturnsEmpty verifies that a bandit with no arms returns "".
func TestEmptyBanditReturnsEmpty(t *testing.T) {
	b := NewBandit()

	if got := b.Pick(); got != "" {
		t.Fatalf("Pick() on empty bandit: expected \"\", got %q", got)
	}
	if got := b.PickWithImpression(); got != "" {
		t.Fatalf("PickWithImpression() on empty bandit: expected \"\", got %q", got)
	}
}

// TestBetaParametersUpdateCorrectly verifies the alpha/beta math after clicks/non-clicks.
func TestBetaParametersUpdateCorrectly(t *testing.T) {
	b := NewBandit()
	b.AddArm("test")

	// Initial: alpha=1, beta=1
	b.checkArm(t, "test", 1, 1)

	// 3 clicks → alpha += 3
	b.Update("test", true)
	b.Update("test", true)
	b.Update("test", true)
	b.checkArm(t, "test", 4, 1)

	// 2 non-clicks → beta += 2
	b.Update("test", false)
	b.Update("test", false)
	b.checkArm(t, "test", 4, 3)
}

// checkArm inspects the internal alpha/beta of an arm.
func (b *Bandit) checkArm(t *testing.T, id string, wantAlpha, wantBeta int) {
	t.Helper()
	arm, ok := b.arms[id]
	if !ok {
		t.Fatalf("arm %q not found", id)
	}
	if int(arm.alpha) != wantAlpha {
		t.Errorf("arm %q alpha = %d, want %d", id, int(arm.alpha), wantAlpha)
	}
	if int(arm.beta) != wantBeta {
		t.Errorf("arm %q beta = %d, want %d", id, int(arm.beta), wantBeta)
	}
}

// TestConcurrentPick verifies no data races under concurrent access.
func TestConcurrentPick(t *testing.T) {
	b := NewBandit()
	for i := 0; i < 5; i++ {
		b.AddArm(fmt.Sprintf("arm-%d", i))
	}

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = b.Pick()
			}
		}()
	}
	wg.Wait()
}

// TestConcurrentPickWithImpression verifies no data races with PickWithImpression.
func TestConcurrentPickWithImpression(t *testing.T) {
	b := NewBandit()
	for i := 0; i < 5; i++ {
		b.AddArm(fmt.Sprintf("arm-%d", i))
	}

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				picked := b.PickWithImpression()
				b.Update(picked, true)
			}
		}()
	}
	wg.Wait()
}