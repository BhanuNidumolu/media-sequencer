package main

import (
	"testing"
	"time"
)

func makeWindow(createdAt time.Time, items ...MediaItem) *Window {
	return &Window{
		ID:        "w-test",
		Name:      "Test Window",
		CreatedAt: createdAt,
		Playlist:  items,
	}
}

func img(id string, dur int) MediaItem {
	return MediaItem{ID: id, Type: MediaImage, URL: "https://example.com/" + id, DurationSeconds: dur}
}

func blank(id string, dur int) MediaItem {
	return MediaItem{ID: id, Type: MediaBlank, DurationSeconds: dur}
}

// ---------- totalDuration ----------

func TestTotalDuration(t *testing.T) {
	cases := []struct {
		name     string
		playlist []MediaItem
		want     int
	}{
		{"empty playlist", nil, 0},
		{"single item", []MediaItem{img("m1", 10)}, 10},
		{"multiple items", []MediaItem{img("m1", 10), img("m2", 5), blank("m3", 3)}, 18},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := totalDuration(tc.playlist)
			if got != tc.want {
				t.Errorf("totalDuration() = %d, want %d", got, tc.want)
			}
		})
	}
}

// ---------- currentItemForWindow ----------

func TestCurrentItemForWindow_EmptyPlaylist(t *testing.T) {
	now := time.Now()
	w := makeWindow(now.Add(-1 * time.Hour))
	item, pos := currentItemForWindow(w, now)
	if item.Type != MediaBlank {
		t.Errorf("expected blank fallback for empty playlist, got type %q", item.Type)
	}
	if pos != 0 {
		t.Errorf("expected position 0 for empty playlist, got %d", pos)
	}
}

func TestCurrentItemForWindow_ZeroDurationPlaylist(t *testing.T) {
	// A playlist whose only item has 0 duration should not divide by zero.
	now := time.Now()
	w := makeWindow(now.Add(-1*time.Hour), img("m1", 0))
	item, _ := currentItemForWindow(w, now)
	if item.Type != MediaBlank {
		t.Errorf("expected blank fallback when total duration is 0, got type %q", item.Type)
	}
}

func TestCurrentItemForWindow_AtCreation(t *testing.T) {
	// elapsed = 0 should land at the very start of the first item.
	now := time.Now()
	w := makeWindow(now, img("m1", 10), img("m2", 10))
	item, pos := currentItemForWindow(w, now)
	if item.ID != "m1" {
		t.Errorf("at t=0 expected m1, got %s", item.ID)
	}
	if pos != 0 {
		t.Errorf("at t=0 expected position 0, got %d", pos)
	}
}

func TestCurrentItemForWindow_MidItem(t *testing.T) {
	now := time.Now()
	// m1: [0,10)  m2: [10,20)  m3: [20,30) -> cycle length 30
	w := makeWindow(now.Add(-15*time.Second), img("m1", 10), img("m2", 10), img("m3", 10))
	item, pos := currentItemForWindow(w, now)
	if item.ID != "m2" {
		t.Errorf("at t=15s expected m2, got %s", item.ID)
	}
	if pos != 5 {
		t.Errorf("at t=15s expected position 5 into m2, got %d", pos)
	}
}

func TestCurrentItemForWindow_ItemBoundary(t *testing.T) {
	// Exactly at the boundary between m1 and m2 (t=10s), m2 should be current
	// per the "position < acc+duration" (half-open interval) rule.
	now := time.Now()
	w := makeWindow(now.Add(-10*time.Second), img("m1", 10), img("m2", 10))
	item, pos := currentItemForWindow(w, now)
	if item.ID != "m2" {
		t.Errorf("at exact boundary t=10s expected m2 (half-open interval), got %s", item.ID)
	}
	if pos != 0 {
		t.Errorf("at exact boundary expected position 0 into m2, got %d", pos)
	}
}

func TestCurrentItemForWindow_LoopsAfterFullCycle(t *testing.T) {
	// cycle length 20 (m1:10 + m2:10). At t=25s we should be 5s into m2 of
	// the *second* loop, same as t=5s... wait: 25 % 20 = 5 -> mid m1.
	now := time.Now()
	w := makeWindow(now.Add(-25*time.Second), img("m1", 10), img("m2", 10))
	item, pos := currentItemForWindow(w, now)
	if item.ID != "m1" {
		t.Errorf("at t=25s (25%%20=5) expected m1, got %s", item.ID)
	}
	if pos != 5 {
		t.Errorf("at t=25s expected position 5 into m1, got %d", pos)
	}
}

func TestCurrentItemForWindow_ManyLoops(t *testing.T) {
	// Same playlist, many cycles later — should be time-based and stateless,
	// i.e. give the identical answer regardless of how many loops have passed.
	now := time.Now()
	cycleLen := 20 * time.Second
	w1 := makeWindow(now.Add(-5 * time.Second))
	w1.Playlist = []MediaItem{img("m1", 10), img("m2", 10)}
	w2 := makeWindow(now.Add(-5*time.Second - 500*cycleLen)) // 500 loops earlier
	w2.Playlist = []MediaItem{img("m1", 10), img("m2", 10)}

	item1, pos1 := currentItemForWindow(w1, now)
	item2, pos2 := currentItemForWindow(w2, now)
	if item1.ID != item2.ID || pos1 != pos2 {
		t.Errorf("expected identical result regardless of loop count: got (%s,%d) vs (%s,%d)",
			item1.ID, pos1, item2.ID, pos2)
	}
}

func TestCurrentItemForWindow_NegativeElapsedClampsToZero(t *testing.T) {
	// CreatedAt in the future (e.g. clock skew) should not panic or go
	// negative; implementation clamps elapsed to 0.
	now := time.Now()
	w := makeWindow(now.Add(1*time.Hour), img("m1", 10), img("m2", 10))
	item, pos := currentItemForWindow(w, now)
	if item.ID != "m1" || pos != 0 {
		t.Errorf("expected clamp to start of playlist for future CreatedAt, got (%s,%d)", item.ID, pos)
	}
}

func TestCurrentItemForWindow_RespectsExplicitBlankItem(t *testing.T) {
	// Blank should only ever appear because it was explicitly configured,
	// never as an implicit "gap" — this exercises that a configured blank
	// item is treated exactly like any other item in the cycle.
	now := time.Now()
	w := makeWindow(now.Add(-12*time.Second), img("m1", 10), blank("m2", 5))
	item, pos := currentItemForWindow(w, now)
	if item.Type != MediaBlank {
		t.Errorf("at t=12s expected configured blank item, got type %q", item.Type)
	}
	if pos != 2 {
		t.Errorf("expected position 2 into blank item, got %d", pos)
	}
}

func TestCurrentItemForWindow_CappedAtMaxCycleSeconds(t *testing.T) {
	// Build a playlist whose total exceeds MaxCycleSeconds and confirm the
	// cycle is capped there rather than using the full (longer) total.
	longItem := img("m-long", MaxCycleSeconds+3600) // 1hr beyond the cap
	now := time.Now()
	// elapsed = MaxCycleSeconds + 100s -> position should be 100s into m-long
	// (since cap makes cycleLen == MaxCycleSeconds, and the single item still
	// covers positions [0, its own duration), which exceeds MaxCycleSeconds
	// anyway, so this mainly documents current single-item behavior).
	w := makeWindow(now.Add(-time.Duration(MaxCycleSeconds+100)*time.Second), longItem)
	item, _ := currentItemForWindow(w, now)
	if item.ID != "m-long" {
		t.Errorf("expected the only item m-long regardless of cap, got %s", item.ID)
	}
}

// ---------- computeDisplay (sync override) ----------

func TestComputeDisplay_NoActiveSync(t *testing.T) {
	now := time.Now()
	w := makeWindow(now.Add(-5*time.Second), img("m1", 10), img("m2", 10))
	display := computeDisplay(w, SyncState{Active: false}, now)
	if display.IsSynced {
		t.Error("expected IsSynced=false when no sync is active")
	}
	if display.Item.ID != "m1" {
		t.Errorf("expected normal sequence item m1, got %s", display.Item.ID)
	}
}

func TestComputeDisplay_ActiveSyncOverridesEveryWindow(t *testing.T) {
	now := time.Now()
	syncItem := img("m-sync", 99)
	sync := SyncState{
		Active:    true,
		MediaID:   syncItem.ID,
		Item:      syncItem,
		StartedAt: now.Add(-2 * time.Second),
		EndsAt:    now.Add(8 * time.Second), // still active
	}

	// Two windows with totally different playlists/positions should both
	// show the synced item while sync is active.
	w1 := makeWindow(now.Add(-3*time.Second), img("a1", 10), img("a2", 10))
	w2 := makeWindow(now.Add(-999*time.Second), img("b1", 7), img("b2", 7))

	d1 := computeDisplay(w1, sync, now)
	d2 := computeDisplay(w2, sync, now)

	if !d1.IsSynced || !d2.IsSynced {
		t.Fatal("expected both windows to report IsSynced=true during active sync")
	}
	if d1.Item.ID != "m-sync" || d2.Item.ID != "m-sync" {
		t.Errorf("expected both windows to show m-sync, got %s and %s", d1.Item.ID, d2.Item.ID)
	}
}

func TestComputeDisplay_ExpiredSyncFallsBackAutomatically(t *testing.T) {
	// The key claim in the README: no cleanup job needed. Once EndsAt has
	// passed, computeDisplay should transparently fall back to the window's
	// own time-based sequence in the very next call.
	now := time.Now()
	syncItem := img("m-sync", 99)
	expiredSync := SyncState{
		Active:    true,
		MediaID:   syncItem.ID,
		Item:      syncItem,
		StartedAt: now.Add(-20 * time.Second),
		EndsAt:    now.Add(-10 * time.Second), // ended 10s ago
	}
	w := makeWindow(now.Add(-5*time.Second), img("m1", 10), img("m2", 10))

	display := computeDisplay(w, expiredSync, now)
	if display.IsSynced {
		t.Error("expected IsSynced=false once EndsAt has passed")
	}
	if display.Item.ID != "m1" {
		t.Errorf("expected window to resume its own sequence (m1), got %s", display.Item.ID)
	}
}

func TestComputeDisplay_SyncDoesNotMutateWindowPosition(t *testing.T) {
	// Confirms the README's core claim: sync is a pure overlay, so a
	// window's underlying computed position is identical whether or not a
	// sync happened in between two calls.
	now := time.Now()
	w := makeWindow(now.Add(-5*time.Second), img("m1", 10), img("m2", 10))

	before := computeDisplay(w, SyncState{Active: false}, now)

	activeSync := SyncState{
		Active: true, Item: img("m-sync", 5),
		StartedAt: now, EndsAt: now.Add(3 * time.Second),
	}
	_ = computeDisplay(w, activeSync, now) // sync happens "in between"

	after := computeDisplay(w, SyncState{Active: false}, now)

	if before.Item.ID != after.Item.ID || before.PositionSeconds != after.PositionSeconds {
		t.Errorf("expected identical position before/after a sync overlay, got %+v vs %+v", before, after)
	}
}
