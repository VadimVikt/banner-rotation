// Package integration provides end-to-end tests for the banner rotation HTTP API.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VadimVikt/banner-rotation/internal/event"
	"github.com/VadimVikt/banner-rotation/internal/handler"
	"github.com/VadimVikt/banner-rotation/internal/repo"
	"github.com/VadimVikt/banner-rotation/internal/service"
	"github.com/go-chi/chi/v5"
)

// setupTestServer creates an in-memory SQLite repo, NOPublisher, service, handlers, and chi router.
func setupTestServer(t *testing.T) (*chi.Mux, *repo.Repo) {
	t.Helper()

	r, err := repo.NewRepo("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("failed to create in-memory repo: %v", err)
	}

	pub := event.NOPublisher{}
	svc := service.NewService(r, pub)

	h := handler.NewHandlers(svc)
	router := chi.NewMux()
	h.RegisterRoutes(router)

	return router, r
}

// jsonBody creates a JSON reader from the given data.
func jsonBody(t *testing.T, data any) io.Reader {
	t.Helper()
	buf, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return bytes.NewReader(buf)
}

// TestEndToEnd_PickAndClick verifies the full pick-and-click flow.
func TestEndToEnd_PickAndClick(t *testing.T) {
	router, r := setupTestServer(t)
	defer func() { _ = r.Close() }()

	slotID := "slot1"
	groupID := "group1"

	// 1. Create slot
	createSlotReq := httptest.NewRequest(http.MethodPost, "/slots", jsonBody(t, map[string]string{
		"id":          slotID,
		"description": "test slot",
	}))
	createSlotReq.Header.Set("Content-Type", "application/json")
	createSlotRec := httptest.NewRecorder()
	router.ServeHTTP(createSlotRec, createSlotReq)
	if createSlotRec.Code != http.StatusCreated {
		t.Fatalf("create slot: expected 201, got %d body: %s", createSlotRec.Code, createSlotRec.Body.String())
	}

	// 2. Add 3 banners
	banners := []string{"banner_a", "banner_b", "banner_c"}
	for _, bannerID := range banners {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/banners", slotID), jsonBody(t, map[string]string{
			"banner_id":   bannerID,
			"description": fmt.Sprintf("banner %s", bannerID),
		}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add banner %s: expected 201, got %d body: %s", bannerID, rec.Code, rec.Body.String())
		}
	}

	// 3. Pick banner 100 times → each banner shown ≥1 times
	pickCounts := map[string]int{}
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/pick?groupID=%s", slotID, groupID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("pick #%d: expected 200, got %d body: %s", i, rec.Code, rec.Body.String())
		}

		var resp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("pick #%d: failed to decode response: %v", i, err)
		}
		pickCounts[resp["banner_id"]]++
	}

	// Verify each banner was shown at least once
	for _, bannerID := range banners {
		if pickCounts[bannerID] == 0 {
			t.Errorf("banner %s was never shown in 100 picks (counts: %v)", bannerID, pickCounts)
		}
	}

	// 4. Register clicks only on banner_a
	clicks := 200
	for i := 0; i < clicks; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/banners/%s/click?groupID=%s", slotID, "banner_a", groupID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("click #%d: expected 204, got %d", i, rec.Code)
		}
	}

	// 5. Pick banner another 200 times → banner_a gets significantly more picks
	secondPickCounts := map[string]int{}
	for i := 0; i < 200; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/pick?groupID=%s", slotID, groupID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("second pick #%d: expected 200, got %d body: %s", i, rec.Code, rec.Body.String())
		}

		var resp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("second pick #%d: failed to decode response: %v", i, err)
		}
		secondPickCounts[resp["banner_id"]]++
	}

	// banner_a should have significantly more picks (at least 50% of total)
	if secondPickCounts["banner_a"] < 100 {
		t.Errorf("banner_a should dominate after clicks: got %d picks out of 200 (counts: %v)",
			secondPickCounts["banner_a"], secondPickCounts)
	}
}

// TestEndToEnd_RemoveBanner verifies that removed banners are never returned.
func TestEndToEnd_RemoveBanner(t *testing.T) {
	router, r := setupTestServer(t)
	defer func() { _ = r.Close() }()

	slotID := "slot1"
	groupID := "group1"

	// 1. Create slot
	createSlotReq := httptest.NewRequest(http.MethodPost, "/slots", jsonBody(t, map[string]string{
		"id":          slotID,
		"description": "test slot",
	}))
	createSlotReq.Header.Set("Content-Type", "application/json")
	createSlotRec := httptest.NewRecorder()
	router.ServeHTTP(createSlotRec, createSlotReq)
	if createSlotRec.Code != http.StatusCreated {
		t.Fatalf("create slot: expected 201, got %d", createSlotRec.Code)
	}

	// 2. Add 2 banners
	for _, bannerID := range []string{"banner_a", "banner_b"} {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/banners", slotID), jsonBody(t, map[string]string{
			"banner_id":   bannerID,
			"description": fmt.Sprintf("banner %s", bannerID),
		}))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("add banner %s: expected 201, got %d", bannerID, rec.Code)
		}
	}

	// 3. Pick banner a few times
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/pick?groupID=%s", slotID, groupID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("pick #%d: expected 200, got %d", i, rec.Code)
		}
	}

	// 4. Remove banner_a
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/slots/%s/banners/%s", slotID, "banner_a"), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove banner: expected 204, got %d", rec.Code)
	}

	// 5. Pick banner again → removed banner should never be returned
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/pick?groupID=%s", slotID, groupID), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("pick after remove #%d: expected 200, got %d", i, rec.Code)
		}

		var resp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("pick after remove #%d: failed to decode response: %v", i, err)
		}
		if resp["banner_id"] == "banner_a" {
			t.Errorf("removed banner_a was returned after removal (pick #%d)", i)
		}
	}
}

// TestEndToEnd_EmptySlot verifies that picking from empty slot returns 404.
func TestEndToEnd_EmptySlot(t *testing.T) {
	router, r := setupTestServer(t)
	defer func() { _ = r.Close() }()

	slotID := "empty_slot"

	// 1. Create slot without adding any banners
	createSlotReq := httptest.NewRequest(http.MethodPost, "/slots", jsonBody(t, map[string]string{
		"id":          slotID,
		"description": "empty slot",
	}))
	createSlotReq.Header.Set("Content-Type", "application/json")
	createSlotRec := httptest.NewRecorder()
	router.ServeHTTP(createSlotRec, createSlotReq)
	if createSlotRec.Code != http.StatusCreated {
		t.Fatalf("create slot: expected 201, got %d", createSlotRec.Code)
	}

	// 2. Try to pick from empty slot → should get 404
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/pick?groupID=group1", slotID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("pick from empty slot: expected 404, got %d body: %s", rec.Code, rec.Body.String())
	}
}

// TestEndToEnd_ClickOnUnknownBanner verifies that clicking on unknown banner returns 404.
func TestEndToEnd_ClickOnUnknownBanner(t *testing.T) {
	router, r := setupTestServer(t)
	defer func() { _ = r.Close() }()

	slotID := "slot1"
	groupID := "group1"

	// 1. Create slot
	createSlotReq := httptest.NewRequest(http.MethodPost, "/slots", jsonBody(t, map[string]string{
		"id":          slotID,
		"description": "test slot",
	}))
	createSlotReq.Header.Set("Content-Type", "application/json")
	createSlotRec := httptest.NewRecorder()
	router.ServeHTTP(createSlotRec, createSlotReq)
	if createSlotRec.Code != http.StatusCreated {
		t.Fatalf("create slot: expected 201, got %d", createSlotRec.Code)
	}

	// 2. Add one banner
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/banners", slotID), jsonBody(t, map[string]string{
		"banner_id":   "banner_a",
		"description": "banner a",
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add banner: expected 201, got %d", rec.Code)
	}

	// 3. Try to click on unknown banner → should get 404
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/slots/%s/banners/%s/click?groupID=%s", slotID, "unknown_banner", groupID), nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("click on unknown banner: expected 404, got %d body: %s", rec.Code, rec.Body.String())
	}
}
