package main

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	return store
}

func TestStore_UpsertAndGetWindow_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	w := &Window{
		ID:        "w1",
		Name:      "Window One",
		CreatedAt: created,
		Playlist: []MediaItem{
			{ID: "m1", Type: MediaImage, URL: "https://example.com/1.jpg", DurationSeconds: 5},
			{ID: "m2", Type: MediaVideo, URL: "https://example.com/2.mp4", DurationSeconds: 12},
		},
	}
	if err := store.UpsertWindow(w); err != nil {
		t.Fatalf("UpsertWindow: %v", err)
	}

	got, ok := store.GetWindow("w1")
	if !ok {
		t.Fatal("expected window w1 to be found after upsert")
	}
	if got.Name != "Window One" || len(got.Playlist) != 2 {
		t.Fatalf("round-tripped window doesn't match: %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt not preserved: got %v, want %v", got.CreatedAt, created)
	}
	// Playlist order must be preserved (position column).
	if got.Playlist[0].ID != "m1" || got.Playlist[1].ID != "m2" {
		t.Errorf("playlist order not preserved: %+v", got.Playlist)
	}
}

func TestStore_UpsertWindow_ReplacesPlaylist(t *testing.T) {
	// UpsertWindow is documented as a full replace, not a merge — confirm
	// old items are actually gone after a second upsert with a different list.
	store := newTestStore(t)
	w := &Window{ID: "w1", Name: "W", Playlist: []MediaItem{
		{ID: "old-1", Type: MediaImage, URL: "https://x/1.jpg", DurationSeconds: 5},
	}}
	if err := store.UpsertWindow(w); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	w.Playlist = []MediaItem{
		{ID: "new-1", Type: MediaImage, URL: "https://x/2.jpg", DurationSeconds: 7},
	}
	if err := store.UpsertWindow(w); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, _ := store.GetWindow("w1")
	if len(got.Playlist) != 1 || got.Playlist[0].ID != "new-1" {
		t.Errorf("expected playlist fully replaced with just new-1, got %+v", got.Playlist)
	}
}

func TestStore_AddMediaToWindow_AppendsAtEnd(t *testing.T) {
	store := newTestStore(t)
	store.UpsertWindow(&Window{ID: "w1", Name: "W", Playlist: []MediaItem{
		{ID: "m1", Type: MediaImage, URL: "https://x/1.jpg", DurationSeconds: 5},
	}})

	updated, err := store.AddMediaToWindow("w1", MediaItem{
		ID: "m2", Type: MediaVideo, URL: "https://x/2.mp4", DurationSeconds: 9,
	})
	if err != nil {
		t.Fatalf("AddMediaToWindow: %v", err)
	}
	if len(updated.Playlist) != 2 {
		t.Fatalf("expected 2 items after add, got %d", len(updated.Playlist))
	}
	if updated.Playlist[0].ID != "m1" || updated.Playlist[1].ID != "m2" {
		t.Errorf("expected new item appended at end, got order %+v", updated.Playlist)
	}
}

func TestStore_AddMediaToWindow_UnknownWindowErrors(t *testing.T) {
	store := newTestStore(t)
	_, err := store.AddMediaToWindow("nope", MediaItem{ID: "m1", Type: MediaImage, URL: "https://x/1.jpg", DurationSeconds: 5})
	if err == nil {
		t.Error("expected an error when adding media to a nonexistent window")
	}
}

func TestStore_AddMediaToWindow_ConcurrentAddsAllPersist(t *testing.T) {
	// Regression test for the store's mutex: concurrent AddMediaToWindow
	// calls against the same window must not lose writes (e.g. via a
	// read-modify-write race on "next position").
	store := newTestStore(t)
	store.UpsertWindow(&Window{ID: "w1", Name: "W", Playlist: nil})

	const n = 20
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			_, err := store.AddMediaToWindow("w1", MediaItem{
				ID: itemID(i), Type: MediaImage, URL: "https://x/img.jpg", DurationSeconds: 1,
			})
			errCh <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent add %d failed: %v", i, err)
		}
	}

	got, _ := store.GetWindow("w1")
	if len(got.Playlist) != n {
		t.Errorf("expected all %d concurrent adds to persist, got %d items", n, len(got.Playlist))
	}
}

func itemID(i int) string {
	return "m-concurrent-" + string(rune('a'+i))
}

func TestStore_SyncState_RoundTrip(t *testing.T) {
	store := newTestStore(t)
	now := time.Now().Truncate(time.Second) // SQLite round-trip via RFC3339Nano string
	state := SyncState{
		Active:    true,
		MediaID:   "m-sync",
		Item:      MediaItem{ID: "m-sync", Type: MediaVideo, URL: "https://x/sync.mp4", DurationSeconds: 20},
		StartedAt: now,
		EndsAt:    now.Add(20 * time.Second),
	}
	if err := store.SetSyncState(state); err != nil {
		t.Fatalf("SetSyncState: %v", err)
	}

	got := store.GetSyncState()
	if !got.Active || got.MediaID != "m-sync" {
		t.Fatalf("expected active sync on m-sync, got %+v", got)
	}
	if !got.EndsAt.Equal(state.EndsAt) {
		t.Errorf("EndsAt not preserved: got %v, want %v", got.EndsAt, state.EndsAt)
	}
}

func TestStore_GetSyncState_NoRowYet(t *testing.T) {
	// Fresh store, sync never triggered — should return a zero-value
	// SyncState (Active=false) rather than erroring.
	store := newTestStore(t)
	got := store.GetSyncState()
	if got.Active {
		t.Errorf("expected Active=false for a fresh store with no sync ever set, got %+v", got)
	}
}

func TestStore_SetSyncState_OverwritesPreviousSync(t *testing.T) {
	store := newTestStore(t)
	now := time.Now()
	store.SetSyncState(SyncState{Active: true, MediaID: "first", StartedAt: now, EndsAt: now.Add(5 * time.Second)})
	store.SetSyncState(SyncState{Active: true, MediaID: "second", StartedAt: now, EndsAt: now.Add(5 * time.Second)})

	got := store.GetSyncState()
	if got.MediaID != "second" {
		t.Errorf("expected latest SetSyncState call to win, got MediaID=%s", got.MediaID)
	}
}
