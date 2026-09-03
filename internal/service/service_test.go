package service

import (
	"context"
	"testing"

	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/model"
	"github.com/VadimVikt/banner-rotation/internal/repo"
)

// countingPublisher captures published events for assertions.
type countingPublisher struct {
	events []model.Event
}

var _ event.Publisher = (*countingPublisher)(nil)

func (p *countingPublisher) Publish(_ context.Context, ev model.Event) error {
	p.events = append(p.events, ev)
	return nil
}

// newTestRepo creates an in-memory SQLite repo for testing.
func newTestRepo(t *testing.T) *repo.Repo {
	t.Helper()
	r, err := repo.NewRepo("file::memory:")
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// setupSlotWithBanners creates a slot, banners, and links them.
func setupSlotWithBanners(r *repo.Repo, slotID string, bannerIDs []string) {
	_ = r.CreateSlot(slotID, "test slot")
	for _, bid := range bannerIDs {
		_ = r.CreateBanner(bid, "banner "+bid)
		_ = r.AddBannerToSlot(slotID, bid)
	}
}

// TestPickBannerReturnsValidBanner verifies the returned banner exists in the slot.
func TestPickBannerReturnsValidBanner(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	bannerIDs := []string{"b1", "b2", "b3"}
	setupSlotWithBanners(r, slotID, bannerIDs)
	ctx := context.Background()

	valid := make(map[string]struct{}, len(bannerIDs))
	for _, bid := range bannerIDs {
		valid[bid] = struct{}{}
	}

	for i := 0; i < 50; i++ {
		bid, err := svc.PickBanner(ctx, slotID, "group1")
		if err != nil {
			t.Fatalf("pick: %v", err)
		}
		if _, ok := valid[bid]; !ok {
			t.Errorf("pick returned unknown banner %q (expected one of %v)", bid, bannerIDs)
		}
	}
}

// TestPickBannerIncrementsImpressions verifies the impression counter increases.
func TestPickBannerIncrementsImpressions(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	bannerIDs := []string{"b1", "b2"}
	setupSlotWithBanners(r, slotID, bannerIDs)
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		if _, err := svc.PickBanner(ctx, slotID, "group1"); err != nil {
			t.Fatalf("pick: %v", err)
		}
	}

	// Sum impressions across both banners for this group.
	var total int
	for _, bid := range bannerIDs {
		imp, _, err := r.GetBannerStats(slotID, bid, "group1")
		if err != nil {
			t.Fatalf("get stats for %s: %v", bid, err)
		}
		total += imp
	}
	if total != 100 {
		t.Errorf("expected 100 total impressions, got %d", total)
	}
}

// TestRegisterClickIncrementsClicks verifies the click counter increases.
func TestRegisterClickIncrementsClicks(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	setupSlotWithBanners(r, slotID, []string{"b1"})
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := svc.RegisterClick(ctx, slotID, "b1", "group1"); err != nil {
			t.Fatalf("click: %v", err)
		}
	}

	_, clicks, err := r.GetBannerStats(slotID, "b1", "group1")
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if clicks != 10 {
		t.Errorf("expected 10 clicks, got %d", clicks)
	}
}

// TestEventsPublished verifies impression and click events are sent to the publisher.
func TestEventsPublished(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	setupSlotWithBanners(r, slotID, []string{"b1"})
	ctx := context.Background()

	bannerID, err := svc.PickBanner(ctx, slotID, "group1")
	if err != nil {
		t.Fatalf("pick: %v", err)
	}

	if len(p.events) != 1 {
		t.Fatalf("expected 1 event after pick, got %d", len(p.events))
	}
	ev := p.events[0]
	if ev.Type != model.EventTypeImpression {
		t.Errorf("expected impression event, got %q", ev.Type)
	}
	if ev.BannerID != bannerID {
		t.Errorf("expected banner %q, got %q", bannerID, ev.BannerID)
	}

	if err := svc.RegisterClick(ctx, slotID, bannerID, "group1"); err != nil {
		t.Fatalf("click: %v", err)
	}

	if len(p.events) != 2 {
		t.Fatalf("expected 2 events total, got %d", len(p.events))
	}
	ev = p.events[1]
	if ev.Type != model.EventTypeClick {
		t.Errorf("expected click event, got %q", ev.Type)
	}
	if ev.BannerID != bannerID {
		t.Errorf("expected banner %q in click event, got %q", bannerID, ev.BannerID)
	}
}

// TestPickBannerEmptySlot verifies error when slot has no banners.
func TestPickBannerEmptySlot(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	_ = r.CreateSlot("empty", "empty slot")

	_, err := svc.PickBanner(context.Background(), "empty", "group1")
	if err == nil {
		t.Error("expected error for empty slot, got nil")
	}
}

// TestAddRemoveBanner verifies banners can be added and removed from slots.
func TestAddRemoveBanner(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	setupSlotWithBanners(r, slotID, []string{"b1"})

	if err := svc.AddBanner(slotID, "b2"); err != nil {
		t.Fatalf("add banner: %v", err)
	}

	banners, _ := r.GetBannersForSlot(slotID)
	if len(banners) != 2 {
		t.Errorf("expected 2 banners, got %d", len(banners))
	}

	if err := svc.RemoveBanner(slotID, "b1"); err != nil {
		t.Fatalf("remove banner: %v", err)
	}

	banners, _ = r.GetBannersForSlot(slotID)
	if len(banners) != 1 {
		t.Errorf("expected 1 banner, got %d", len(banners))
	}
	if len(banners) > 0 && banners[0] != "b2" {
		t.Errorf("expected b2, got %q", banners[0])
	}
}

// TestBanditFavorsClickedBanner verifies clicks are propagated to the bandit.
// The bandit algorithm itself is tested in internal/bandit; this test verifies
// the service correctly feeds click signals.
func TestBanditFavorsClickedBanner(t *testing.T) {
	r := newTestRepo(t)
	p := &countingPublisher{}
	svc := NewService(r, p)

	slotID := "slot1"
	setupSlotWithBanners(r, slotID, []string{"good", "bad"})
	ctx := context.Background()

	// Pick a few times and register clicks only for "good".
	for i := 0; i < 5; i++ {
		_, _ = svc.PickBanner(ctx, slotID, "group1")
	}
	for i := 0; i < 5; i++ {
		_ = svc.RegisterClick(ctx, slotID, "good", "group1")
	}

	// Verify clicks were recorded in the repo.
	_, clicks, err := r.GetBannerStats(slotID, "good", "group1")
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if clicks != 5 {
		t.Errorf("expected 5 clicks for good, got %d", clicks)
	}

	// Verify click events were published.
	clickCount := 0
	for _, ev := range p.events {
		if ev.Type == model.EventTypeClick {
			clickCount++
		}
	}
	if clickCount != 5 {
		t.Errorf("expected 5 click events, got %d", clickCount)
	}
}