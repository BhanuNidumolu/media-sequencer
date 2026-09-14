package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo/C toolchain needed
)

// Store is a thread-safe, SQLite-backed persistence layer.
//
// Design choice: SQLite instead of a client/server database engine like
// Postgres. It's a real relational database - proper tables, SQL queries,
// ACID transactions - but it's a single embedded file, so there's no
// separate database server to install, run, or connect to over a network.
// That keeps local setup and deployment simple while still being an actual
// database rather than a flat file. The tradeoff (same as any embedded
// database) is that it doesn't support multiple backend instances writing
// concurrently the way a client/server database would; if this needed to
// scale that way, swapping to Postgres would only mean changing the SQL
// queries and connection setup in this file - the handlers and sync logic
// only depend on Store's public methods, not how they're implemented.
type Store struct {
	db *sql.DB
	// mu serializes the read-then-write sequences in AddMediaToWindow and
	// UpsertWindow (multiple statements that must be seen as one unit).
	// SQLite itself only allows one writer at a time regardless, but this
	// mutex keeps each such operation atomic from the app's point of view too.
	mu sync.Mutex
}

// NewStore opens (creating if necessary) the SQLite database at filePath
// and ensures its schema exists.
func NewStore(filePath string) (*Store, error) {
	if dir := filepath.Dir(filePath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating data directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", filePath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite db: %w", err)
	}
	// SQLite allows only one writer at a time; capping the pool to a
	// single connection avoids "database is locked" errors under
	// concurrent requests rather than fighting them with retries.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrating schema: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS windows (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS media_items (
			id               TEXT PRIMARY KEY,
			window_id        TEXT NOT NULL REFERENCES windows(id),
			position         INTEGER NOT NULL,
			type             TEXT NOT NULL,
			url              TEXT NOT NULL DEFAULT '',
			duration_seconds INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_media_items_window
			ON media_items(window_id, position);
		CREATE TABLE IF NOT EXISTS sync_state (
			id                    INTEGER PRIMARY KEY CHECK (id = 1),
			active                INTEGER NOT NULL DEFAULT 0,
			media_id              TEXT,
			item_type             TEXT,
			item_url              TEXT,
			item_duration_seconds INTEGER,
			started_at            TEXT,
			ends_at               TEXT
		);
	`)
	return err
}

// AllWindows returns every window with its full playlist, ordered by
// each item's position within its window.
func (s *Store) AllWindows() []*Window {
	s.mu.Lock()
	defer s.mu.Unlock()
	windows, err := s.allWindowsLocked()
	if err != nil {
		// Store's methods don't return errors here to keep the same
		// signature as the original file-backed version; a query failure
		// against a healthy local SQLite file would indicate something
		// seriously wrong (e.g. disk failure), so we log and return empty
		// rather than panic.
		log.Println("AllWindows error:", err)
		return nil
	}
	return windows
}

func (s *Store) allWindowsLocked() ([]*Window, error) {
	rows, err := s.db.Query(`SELECT id, name, created_at FROM windows ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byID := make(map[string]*Window)
	order := make([]string, 0)
	for rows.Next() {
		var w Window
		var createdAt string
		if err := rows.Scan(&w.ID, &w.Name, &createdAt); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, err
		}
		w.CreatedAt = t
		w.Playlist = []MediaItem{}
		byID[w.ID] = &w
		order = append(order, w.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	itemRows, err := s.db.Query(`
		SELECT window_id, id, type, url, duration_seconds
		FROM media_items ORDER BY window_id, position`)
	if err != nil {
		return nil, err
	}
	defer itemRows.Close()

	for itemRows.Next() {
		var windowID string
		var item MediaItem
		if err := itemRows.Scan(&windowID, &item.ID, &item.Type, &item.URL, &item.DurationSeconds); err != nil {
			return nil, err
		}
		if w, ok := byID[windowID]; ok {
			w.Playlist = append(w.Playlist, item)
		}
	}
	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	out := make([]*Window, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// GetWindow returns a single window by ID.
func (s *Store) GetWindow(id string) (*Window, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	windows, err := s.allWindowsLocked()
	if err != nil {
		log.Println("GetWindow error:", err)
		return nil, false
	}
	for _, w := range windows {
		if w.ID == id {
			return w, true
		}
	}
	return nil, false
}

// UpsertWindow creates or fully replaces a window and its playlist.
func (s *Store) UpsertWindow(w *Window) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op if Commit already succeeded

	_, err = tx.Exec(`
		INSERT INTO windows (id, name, created_at) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, created_at = excluded.created_at
	`, w.ID, w.Name, w.CreatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("upserting window: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM media_items WHERE window_id = ?`, w.ID); err != nil {
		return fmt.Errorf("clearing old playlist: %w", err)
	}
	for i, item := range w.Playlist {
		if _, err := tx.Exec(`
			INSERT INTO media_items (id, window_id, position, type, url, duration_seconds)
			VALUES (?, ?, ?, ?, ?, ?)
		`, item.ID, w.ID, i, string(item.Type), item.URL, item.DurationSeconds); err != nil {
			return fmt.Errorf("inserting playlist item: %w", err)
		}
	}

	return tx.Commit()
}

// AddMediaToWindow appends one item to the end of a window's playlist.
func (s *Store) AddMediaToWindow(windowID string, item MediaItem) (*Window, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var exists int
	if err := tx.QueryRow(`SELECT COUNT(1) FROM windows WHERE id = ?`, windowID).Scan(&exists); err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, fmt.Errorf("window %q not found", windowID)
	}

	var nextPosition int
	if err := tx.QueryRow(`
		SELECT COALESCE(MAX(position), -1) + 1 FROM media_items WHERE window_id = ?
	`, windowID).Scan(&nextPosition); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
		INSERT INTO media_items (id, window_id, position, type, url, duration_seconds)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ID, windowID, nextPosition, string(item.Type), item.URL, item.DurationSeconds); err != nil {
		return nil, fmt.Errorf("inserting media item: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	windows, err := s.allWindowsLocked()
	if err != nil {
		return nil, err
	}
	for _, w := range windows {
		if w.ID == windowID {
			return w, nil
		}
	}
	return nil, fmt.Errorf("window %q not found after insert", windowID)
}

// SetSyncState overwrites the current sync event (a single-row table).
func (s *Store) SetSyncState(state SyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	active := 0
	if state.Active {
		active = 1
	}
	var startedAt, endsAt sql.NullString
	if !state.StartedAt.IsZero() {
		startedAt = sql.NullString{String: state.StartedAt.Format(time.RFC3339Nano), Valid: true}
	}
	if !state.EndsAt.IsZero() {
		endsAt = sql.NullString{String: state.EndsAt.Format(time.RFC3339Nano), Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT INTO sync_state (id, active, media_id, item_type, item_url, item_duration_seconds, started_at, ends_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			active = excluded.active,
			media_id = excluded.media_id,
			item_type = excluded.item_type,
			item_url = excluded.item_url,
			item_duration_seconds = excluded.item_duration_seconds,
			started_at = excluded.started_at,
			ends_at = excluded.ends_at
	`, active, state.MediaID, string(state.Item.Type), state.Item.URL, state.Item.DurationSeconds, startedAt, endsAt)
	return err
}

// GetSyncState returns the current sync event (zero-value if none has ever been set).
func (s *Store) GetSyncState() SyncState {
	s.mu.Lock()
	defer s.mu.Unlock()

	var active int
	var mediaID, itemType, itemURL sql.NullString
	var itemDuration sql.NullInt64
	var startedAt, endsAt sql.NullString

	err := s.db.QueryRow(`
		SELECT active, media_id, item_type, item_url, item_duration_seconds, started_at, ends_at
		FROM sync_state WHERE id = 1
	`).Scan(&active, &mediaID, &itemType, &itemURL, &itemDuration, &startedAt, &endsAt)
	if err != nil {
		// No row yet (fresh database) - no sync has ever been triggered.
		return SyncState{}
	}

	state := SyncState{
		Active:  active == 1,
		MediaID: mediaID.String,
		Item: MediaItem{
			ID:              mediaID.String,
			Type:            MediaType(itemType.String),
			URL:             itemURL.String,
			DurationSeconds: int(itemDuration.Int64),
		},
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, startedAt.String); err == nil {
			state.StartedAt = t
		}
	}
	if endsAt.Valid {
		if t, err := time.Parse(time.RFC3339Nano, endsAt.String); err == nil {
			state.EndsAt = t
		}
	}
	return state
}
