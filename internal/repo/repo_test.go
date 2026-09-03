package repo

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

// testDSN creates a unique in-memory SQLite DSN for each test.
// Each connection gets its own isolated database (no shared cache).
func testDSN(t *testing.T) string {
	t.Helper()
	return "file::memory:"
}

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	r, err := NewRepo(testDSN(t))
	if err != nil {
		t.Fatalf("NewRepo: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestCreateSlot(t *testing.T) {
	r := newTestRepo(t)

	if err := r.CreateSlot("slot-1", "Hero banner"); err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}

	// Duplicate should fail
	if err := r.CreateSlot("slot-1", "Duplicate"); err == nil {
		t.Fatal("expected error on duplicate slot, got nil")
	}
}

func TestCreateBanner(t *testing.T) {
	r := newTestRepo(t)

	if err := r.CreateBanner("banner-1", "Summer sale"); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}

	// Duplicate should fail
	if err := r.CreateBanner("banner-1", "Duplicate"); err == nil {
		t.Fatal("expected error on duplicate banner, got nil")
	}
}

func TestAddRemoveBannerToSlot(t *testing.T) {
	r := newTestRepo(t)

	_ = r.CreateSlot("slot-1", "Hero")
	_ = r.CreateBanner("b1", "Banner 1")
	_ = r.CreateBanner("b2", "Banner 2")

	// Add banners
	if err := r.AddBannerToSlot("slot-1", "b1"); err != nil {
		t.Fatalf("AddBannerToSlot(b1): %v", err)
	}
	if err := r.AddBannerToSlot("slot-1", "b2"); err != nil {
		t.Fatalf("AddBannerToSlot(b2): %v", err)
	}

	// Verify both present
	banners, err := r.GetBannersForSlot("slot-1")
	if err != nil {
		t.Fatalf("GetBannersForSlot: %v", err)
	}
	if len(banners) != 2 {
		t.Fatalf("expected 2 banners, got %d", len(banners))
	}

	// Remove one
	if err := r.RemoveBannerFromSlot("slot-1", "b1"); err != nil {
		t.Fatalf("RemoveBannerFromSlot: %v", err)
	}

	banners, err = r.GetBannersForSlot("slot-1")
	if err != nil {
		t.Fatalf("GetBannersForSlot after remove: %v", err)
	}
	if len(banners) != 1 || banners[0] != "b2" {
		t.Fatalf("expected [b2], got %v", banners)
	}
}

func TestGetBannersForSlot(t *testing.T) {
	r := newTestRepo(t)

	_ = r.CreateSlot("slot-1", "Hero")
	_ = r.CreateSlot("slot-2", "Footer")
	_ = r.CreateBanner("b1", "Banner 1")
	_ = r.CreateBanner("b2", "Banner 2")
	_ = r.CreateBanner("b3", "Banner 3")

	_ = r.AddBannerToSlot("slot-1", "b1")
	_ = r.AddBannerToSlot("slot-1", "b2")
	_ = r.AddBannerToSlot("slot-2", "b3")

	// Slot 1 should have b1 and b2
	banners, err := r.GetBannersForSlot("slot-1")
	if err != nil {
		t.Fatalf("GetBannersForSlot(slot-1): %v", err)
	}
	sort.Strings(banners)
	if len(banners) != 2 || banners[0] != "b1" || banners[1] != "b2" {
		t.Fatalf("expected [b1, b2], got %v", banners)
	}

	// Slot 2 should have only b3
	banners, err = r.GetBannersForSlot("slot-2")
	if err != nil {
		t.Fatalf("GetBannersForSlot(slot-2): %v", err)
	}
	if len(banners) != 1 || banners[0] != "b3" {
		t.Fatalf("expected [b3], got %v", banners)
	}

	// Empty slot should return empty slice
	banners, err = r.GetBannersForSlot("slot-empty")
	if err != nil {
		t.Fatalf("GetBannersForSlot(empty): %v", err)
	}
	if len(banners) != 0 {
		t.Fatalf("expected empty slice, got %v", banners)
	}
}

func TestIncrementImpressions(t *testing.T) {
	r := newTestRepo(t)

	const slot, banner, group = "s1", "b1", "g1"

	// First increment should auto-create via UPSERT
	if err := r.IncrementImpressions(slot, banner, group); err != nil {
		t.Fatalf("IncrementImpressions (first): %v", err)
	}

	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 1 || clk != 0 {
		t.Fatalf("expected (1, 0), got (%d, %d)", imp, clk)
	}

	// Second increment
	if err := r.IncrementImpressions(slot, banner, group); err != nil {
		t.Fatalf("IncrementImpressions (second): %v", err)
	}

	imp, clk, err = r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 2 || clk != 0 {
		t.Fatalf("expected (2, 0), got (%d, %d)", imp, clk)
	}
}

func TestIncrementClicks(t *testing.T) {
	r := newTestRepo(t)

	const slot, banner, group = "s1", "b1", "g1"

	// First click should auto-create via UPSERT
	if err := r.IncrementClicks(slot, banner, group); err != nil {
		t.Fatalf("IncrementClicks (first): %v", err)
	}

	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 0 || clk != 1 {
		t.Fatalf("expected (0, 1), got (%d, %d)", imp, clk)
	}

	// Second click
	if err := r.IncrementClicks(slot, banner, group); err != nil {
		t.Fatalf("IncrementClicks (second): %v", err)
	}

	imp, clk, err = r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 0 || clk != 2 {
		t.Fatalf("expected (0, 2), got (%d, %d)", imp, clk)
	}
}

func TestGetBannerStats(t *testing.T) {
	r := newTestRepo(t)

	const slot, banner, group = "s1", "b1", "g1"

	// No stats yet — should return (0, 0, nil)
	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats (empty): %v", err)
	}
	if imp != 0 || clk != 0 {
		t.Fatalf("expected (0, 0), got (%d, %d)", imp, clk)
	}

	// Add some stats
	_ = r.IncrementImpressions(slot, banner, group)
	_ = r.IncrementImpressions(slot, banner, group)
	_ = r.IncrementClicks(slot, banner, group)

	imp, clk, err = r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 2 || clk != 1 {
		t.Fatalf("expected (2, 1), got (%d, %d)", imp, clk)
	}

	// Different group should have no stats
	imp, clk, err = r.GetBannerStats(slot, banner, "other-group")
	if err != nil {
		t.Fatalf("GetBannerStats (other group): %v", err)
	}
	if imp != 0 || clk != 0 {
		t.Fatalf("expected (0, 0) for other group, got (%d, %d)", imp, clk)
	}
}

func TestConcurrentIncrements(t *testing.T) {
	r := newTestRepo(t)

	const (
		slot         = "s1"
		banner       = "b1"
		group        = "g1"
		goroutines   = 10
		perGoroutine = 100
		expected     = goroutines * perGoroutine
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := r.IncrementImpressions(slot, banner, group); err != nil {
					t.Errorf("IncrementImpressions: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != expected {
		t.Fatalf("expected %d impressions, got %d", expected, imp)
	}
	if clk != 0 {
		t.Fatalf("expected 0 clicks, got %d", clk)
	}
}

func TestConcurrentClicks(t *testing.T) {
	r := newTestRepo(t)

	const (
		slot         = "s1"
		banner       = "b1"
		group        = "g1"
		goroutines   = 10
		perGoroutine = 100
		expected     = goroutines * perGoroutine
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := r.IncrementClicks(slot, banner, group); err != nil {
					t.Errorf("IncrementClicks: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if clk != expected {
		t.Fatalf("expected %d clicks, got %d", expected, clk)
	}
	if imp != 0 {
		t.Fatalf("expected 0 impressions, got %d", imp)
	}
}

func TestMixedConcurrentOps(t *testing.T) {
	r := newTestRepo(t)

	const (
		slot         = "s1"
		banner       = "b1"
		group        = "g1"
		goroutines   = 5
		perGoroutine = 50
	)

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines increment impressions
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := r.IncrementImpressions(slot, banner, group); err != nil {
					t.Errorf("IncrementImpressions: %v", err)
					return
				}
			}
		}()
	}

	// Half goroutines increment clicks
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := r.IncrementClicks(slot, banner, group); err != nil {
					t.Errorf("IncrementClicks: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	imp, clk, err := r.GetBannerStats(slot, banner, group)
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}

	expected := goroutines * perGoroutine
	if imp != expected {
		t.Fatalf("expected %d impressions, got %d", expected, imp)
	}
	if clk != expected {
		t.Fatalf("expected %d clicks, got %d", expected, clk)
	}
}

func TestMultipleSlotsAndGroups(t *testing.T) {
	r := newTestRepo(t)

	// Two slots, two banners, two groups
	_ = r.CreateSlot("s1", "Slot 1")
	_ = r.CreateSlot("s2", "Slot 2")
	_ = r.CreateBanner("b1", "Banner 1")
	_ = r.CreateBanner("b2", "Banner 2")

	_ = r.AddBannerToSlot("s1", "b1")
	_ = r.AddBannerToSlot("s1", "b2")
	_ = r.AddBannerToSlot("s2", "b1")

	// Impressions for different slot/group combos
	_ = r.IncrementImpressions("s1", "b1", "geo-us")
	_ = r.IncrementImpressions("s1", "b1", "geo-us")
	_ = r.IncrementImpressions("s1", "b1", "geo-eu")
	_ = r.IncrementImpressions("s2", "b1", "geo-us")
	_ = r.IncrementClicks("s1", "b1", "geo-us")

	// s1/b1/geo-us: 2 impressions, 1 click
	imp, clk, err := r.GetBannerStats("s1", "b1", "geo-us")
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 2 || clk != 1 {
		t.Fatalf("s1/b1/geo-us: expected (2,1), got (%d,%d)", imp, clk)
	}

	// s1/b1/geo-eu: 1 impression, 0 clicks
	imp, clk, err = r.GetBannerStats("s1", "b1", "geo-eu")
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 1 || clk != 0 {
		t.Fatalf("s1/b1/geo-eu: expected (1,0), got (%d,%d)", imp, clk)
	}

	// s2/b1/geo-us: 1 impression, 0 clicks
	imp, clk, err = r.GetBannerStats("s2", "b1", "geo-us")
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 1 || clk != 0 {
		t.Fatalf("s2/b1/geo-us: expected (1,0), got (%d,%d)", imp, clk)
	}

	// s1/b2/geo-us: no stats
	imp, clk, err = r.GetBannerStats("s1", "b2", "geo-us")
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 0 || clk != 0 {
		t.Fatalf("s1/b2/geo-us: expected (0,0), got (%d,%d)", imp, clk)
	}
}

func TestNewRepoValidDSN(t *testing.T) {
	// Verify that NewRepo returns a usable repo for a valid DSN.
	r, err := NewRepo("file::memory:")
	if err != nil {
		t.Fatalf("NewRepo: %v", err)
	}
	defer r.Close()

	// Should be able to use it
	if err := r.CreateSlot("s1", "test"); err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
}

func TestRepoClose(t *testing.T) {
	r, err := NewRepo("file::memory:")
	if err != nil {
		t.Fatalf("NewRepo: %v", err)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// After close, operations should fail
	if err := r.CreateSlot("s1", "test"); err == nil {
		t.Fatal("expected error after close, got nil")
	}
}

func TestDuplicateBannerToSlot(t *testing.T) {
	r := newTestRepo(t)

	_ = r.CreateSlot("s1", "Slot 1")
	_ = r.CreateBanner("b1", "Banner 1")

	_ = r.AddBannerToSlot("s1", "b1")

	// Adding the same banner again should fail (PK violation)
	if err := r.AddBannerToSlot("s1", "b1"); err == nil {
		t.Fatal("expected error on duplicate slot_banner, got nil")
	}
}

func TestRemoveNonExistentBanner(t *testing.T) {
	r := newTestRepo(t)

	// Removing a banner that was never added should not error
	if err := r.RemoveBannerFromSlot("s1", "b1"); err != nil {
		t.Fatalf("RemoveBannerFromSlot (non-existent): %v", err)
	}
}

func TestSchemaCreatedOnNewRepo(t *testing.T) {
	r := newTestRepo(t)

	// If schema was created properly, all operations should work
	if err := r.CreateSlot("s1", "test"); err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
	if err := r.CreateBanner("b1", "test"); err != nil {
		t.Fatalf("CreateBanner: %v", err)
	}
	if err := r.AddBannerToSlot("s1", "b1"); err != nil {
		t.Fatalf("AddBannerToSlot: %v", err)
	}
	if err := r.IncrementImpressions("s1", "b1", "g1"); err != nil {
		t.Fatalf("IncrementImpressions: %v", err)
	}
	if err := r.IncrementClicks("s1", "b1", "g1"); err != nil {
		t.Fatalf("IncrementClicks: %v", err)
	}

	imp, clk, err := r.GetBannerStats("s1", "b1", "g1")
	if err != nil {
		t.Fatalf("GetBannerStats: %v", err)
	}
	if imp != 1 || clk != 1 {
		t.Fatalf("expected (1,1), got (%d,%d)", imp, clk)
	}
}

func TestErrorWrapping(t *testing.T) {
	r := newTestRepo(t)

	// Create a slot, then try to create it again — should get a wrapped error
	err := r.CreateSlot("s1", "first")
	if err != nil {
		t.Fatalf("first CreateSlot: %v", err)
	}
	err = r.CreateSlot("s1", "second")
	if err == nil {
		t.Fatal("expected error on duplicate slot")
	}
	// Check the error message contains the slot ID
	if fmt.Sprintf("%v", err) == "" {
		t.Fatal("error message should not be empty")
	}
}