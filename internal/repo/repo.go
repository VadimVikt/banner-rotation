// Package repo provides a SQLite-backed repository for slots, banners, and banner statistics.
package repo

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite" // SQLite driver (pure Go, no CGO)
)

const schema = `
CREATE TABLE IF NOT EXISTS slots (
	id TEXT PRIMARY KEY,
	description TEXT
);

CREATE TABLE IF NOT EXISTS banners (
	id TEXT PRIMARY KEY,
	description TEXT
);

CREATE TABLE IF NOT EXISTS slot_banners (
	slot_id   TEXT NOT NULL,
	banner_id TEXT NOT NULL,
	PRIMARY KEY (slot_id, banner_id)
);

CREATE TABLE IF NOT EXISTS banner_stats (
	slot_id     TEXT NOT NULL,
	banner_id   TEXT NOT NULL,
	group_id    TEXT NOT NULL,
	impressions INTEGER DEFAULT 0,
	clicks      INTEGER DEFAULT 0,
	PRIMARY KEY (slot_id, banner_id, group_id)
);
`

// Repo wraps a *sql.DB and provides CRUD operations for slots, banners, and banner statistics.
type Repo struct {
	db *sql.DB
	mu sync.Mutex
}

// NewRepo opens a SQLite database at the given DSN and runs schema migrations.
// Example DSN for file-based: "file:/path/to/db.sqlite?_journal_mode=WAL"
// Example DSN for in-memory:  "file::memory:"
func NewRepo(dsn string) (*Repo, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Verify the connection is alive.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	r := &Repo{db: db}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("execute schema: %w", err)
	}

	return r, nil
}

// Close closes the underlying database connection.
func (r *Repo) Close() error {
	return r.db.Close()
}

// CreateSlot inserts a new slot. Returns an error if the slot already exists.
func (r *Repo) CreateSlot(id, desc string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec("INSERT INTO slots (id, description) VALUES (?, ?)", id, desc)
	if err != nil {
		return fmt.Errorf("insert slot %q: %w", id, err)
	}
	return nil
}

// CreateBanner inserts a new banner. Returns an error if the banner already exists.
func (r *Repo) CreateBanner(id, desc string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec("INSERT INTO banners (id, description) VALUES (?, ?)", id, desc)
	if err != nil {
		return fmt.Errorf("insert banner %q: %w", id, err)
	}
	return nil
}

// AddBannerToSlot links a banner to a slot.
func (r *Repo) AddBannerToSlot(slotID, bannerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec("INSERT INTO slot_banners (slot_id, banner_id) VALUES (?, ?)", slotID, bannerID)
	if err != nil {
		return fmt.Errorf("add banner %q to slot %q: %w", bannerID, slotID, err)
	}
	return nil
}

// RemoveBannerFromSlot unlinks a banner from a slot.
func (r *Repo) RemoveBannerFromSlot(slotID, bannerID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec("DELETE FROM slot_banners WHERE slot_id = ? AND banner_id = ?", slotID, bannerID)
	if err != nil {
		return fmt.Errorf("remove banner %q from slot %q: %w", bannerID, slotID, err)
	}
	return nil
}

// GetBannersForSlot returns all banner IDs associated with the given slot.
func (r *Repo) GetBannersForSlot(slotID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.db.Query("SELECT banner_id FROM slot_banners WHERE slot_id = ?", slotID)
	if err != nil {
		return nil, fmt.Errorf("query banners for slot %q: %w", slotID, err)
	}
	defer rows.Close()

	var banners []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan banner id: %w", err)
		}
		banners = append(banners, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate banners: %w", err)
	}

	return banners, nil
}

// GetBannerStats returns impressions and clicks for a specific slot/banner/group combination.
// Returns (0, 0, nil) if no record exists.
func (r *Repo) GetBannerStats(slotID, bannerID, groupID string) (impressions, clicks int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	row := r.db.QueryRow(
		"SELECT impressions, clicks FROM banner_stats WHERE slot_id = ? AND banner_id = ? AND group_id = ?",
		slotID, bannerID, groupID,
	)

	switch err := row.Scan(&impressions, &clicks); {
	case err == sql.ErrNoRows:
		return 0, 0, nil
	case err != nil:
		return 0, 0, fmt.Errorf("query banner stats (%s/%s/%s): %w", slotID, bannerID, groupID, err)
	default:
		return impressions, clicks, nil
	}
}

// IncrementImpressions atomically increments the impression counter for a slot/banner/group.
// Uses UPSERT to create the row if it does not exist.
func (r *Repo) IncrementImpressions(slotID, bannerID, groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec(`
		INSERT INTO banner_stats (slot_id, banner_id, group_id, impressions, clicks)
		VALUES (?, ?, ?, 1, 0)
		ON CONFLICT(slot_id, banner_id, group_id)
		DO UPDATE SET impressions = impressions + 1
	`, slotID, bannerID, groupID)
	if err != nil {
		return fmt.Errorf("increment impressions (%s/%s/%s): %w", slotID, bannerID, groupID, err)
	}
	return nil
}

// IncrementClicks atomically increments the click counter for a slot/banner/group.
// Uses UPSERT to create the row if it does not exist.
func (r *Repo) IncrementClicks(slotID, bannerID, groupID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, err := r.db.Exec(`
		INSERT INTO banner_stats (slot_id, banner_id, group_id, impressions, clicks)
		VALUES (?, ?, ?, 0, 1)
		ON CONFLICT(slot_id, banner_id, group_id)
		DO UPDATE SET clicks = clicks + 1
	`, slotID, bannerID, groupID)
	if err != nil {
		return fmt.Errorf("increment clicks (%s/%s/%s): %w", slotID, bannerID, groupID, err)
	}
	return nil
}