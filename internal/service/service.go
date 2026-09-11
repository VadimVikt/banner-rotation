// Package service orchestrates banner selection using the multi-armed bandit algorithm.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/VadimVikt/banner-rotation/internal/bandit"
	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/model"
	"github.com/VadimVikt/banner-rotation/internal/repo"
)

// ErrNoBanners is returned when a slot has no banners.
var ErrNoBanners = errors.New("slot has no banners")

// ErrUnknownBanner is returned when a banner is not found in the slot.
var ErrUnknownBanner = errors.New("banner not found in slot")

// Service orchestrates banner selection using multi-armed bandit per slot+group.
type Service struct {
	repo      repo.RepoInterface
	publisher event.Publisher
	bandits   map[string]*bandit.Bandit
	mu        sync.Mutex
}

// NewService creates a new Service.
func NewService(r repo.RepoInterface, p event.Publisher) *Service {
	return &Service{
		repo:      r,
		publisher: p,
		bandits:   make(map[string]*bandit.Bandit),
	}
}

func (s *Service) getBandit(slotID, groupID string) *bandit.Bandit {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := slotID + ":" + groupID
	b := s.bandits[key]
	if b == nil {
		b = bandit.NewBandit()
		s.bandits[key] = b
	}
	return b
}

func (s *Service) syncArms(b *bandit.Bandit, bannerIDs []string) {
	for _, id := range bannerIDs {
		b.AddArm(id)
	}
}

func (s *Service) CreateSlot(id, desc string) error {
	return s.repo.CreateSlot(id, desc)
}

func (s *Service) CreateBanner(id, desc string) error {
	return s.repo.CreateBanner(id, desc)
}

func (s *Service) AddBanner(slotID, bannerID string) error {
	return s.repo.AddBannerToSlot(slotID, bannerID)
}

// AddBannerWithDescription creates the banner in the repository and adds it to the slot.
func (s *Service) AddBannerWithDescription(slotID, bannerID, desc string) error {
	if err := s.repo.CreateBanner(bannerID, desc); err != nil {
		return fmt.Errorf("create banner %q: %w", bannerID, err)
	}
	return s.repo.AddBannerToSlot(slotID, bannerID)
}

func (s *Service) RemoveBanner(slotID, bannerID string) error {
	return s.repo.RemoveBannerFromSlot(slotID, bannerID)
}

func (s *Service) PickBanner(ctx context.Context, slotID, groupID string) (string, error) {
	banners, err := s.repo.GetBannersForSlot(slotID)
	if err != nil {
		return "", fmt.Errorf("get banners for slot %q: %w", slotID, err)
	}
	if len(banners) == 0 {
		return "", ErrNoBanners
	}
	b := s.getBandit(slotID, groupID)
	s.syncArms(b, banners)
	bannerID := b.PickWithImpression()
	if bannerID == "" {
		return "", ErrNoBanners
	}
	if err := s.repo.IncrementImpressions(slotID, bannerID, groupID); err != nil {
		return "", fmt.Errorf("increment impressions: %w", err)
	}
	ev := model.Event{
		Type:      model.EventTypeImpression,
		SlotID:    slotID,
		BannerID:  bannerID,
		GroupID:   model.GroupID(groupID),
		Timestamp: time.Now(),
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		return "", fmt.Errorf("publish impression: %w", err)
	}
	return bannerID, nil
}

func (s *Service) RegisterClick(ctx context.Context, slotID, bannerID, groupID string) error {
	banners, err := s.repo.GetBannersForSlot(slotID)
	if err != nil {
		return fmt.Errorf("get banners for slot %q: %w", slotID, err)
	}
	found := false
	for _, id := range banners {
		if id == bannerID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s in slot %s", ErrUnknownBanner, bannerID, slotID)
	}

	if err := s.repo.IncrementClicks(slotID, bannerID, groupID); err != nil {
		return fmt.Errorf("increment clicks: %w", err)
	}
	b := s.getBandit(slotID, groupID)
	b.Update(bannerID, true)
	ev := model.Event{
		Type:      model.EventTypeClick,
		SlotID:    slotID,
		BannerID:  bannerID,
		GroupID:   model.GroupID(groupID),
		Timestamp: time.Now(),
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		return fmt.Errorf("publish click: %w", err)
	}
	return nil
}
