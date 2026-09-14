package main

import "time"

// totalDuration sums the DurationSeconds of every item in a playlist.
func totalDuration(playlist []MediaItem) int {
	total := 0
	for _, item := range playlist {
		total += item.DurationSeconds
	}
	return total
}

// currentItemForWindow computes which playlist item a window should be
// showing right now, based purely on elapsed wall-clock time since the
// window was created. Because this is derived from time rather than from
// stored "current index" counters, it is:
//   - stateless and restart-safe (a server restart loses no playback position)
//   - trivially consistent if multiple frontend clients ask at once
//
// Algorithm:
//  1. elapsed = seconds since the window was created
//  2. cycleLen = total playlist duration, capped conceptually at 5 hours
//     (MaxCycleSeconds) per the spec's "total play size ... 5 hours" rule
//  3. position = elapsed mod cycleLen  -> where we are inside one loop
//  4. walk the playlist accumulating durations until position falls
//     inside an item; that item is "current"
func currentItemForWindow(w *Window, now time.Time) (MediaItem, int) {
	if len(w.Playlist) == 0 {
		return MediaItem{Type: MediaBlank, DurationSeconds: 0}, 0
	}

	total := totalDuration(w.Playlist)
	if total <= 0 {
		return MediaItem{Type: MediaBlank, DurationSeconds: 0}, 0
	}

	// The playlist loops on its own natural length. We additionally cap
	// the elapsed time at MaxCycleSeconds before taking the modulus so
	// that, per spec, the window's cycle is explicitly bounded at 5 hours
	// even if a future playlist were long enough for that to matter.
	elapsed := int(now.Sub(w.CreatedAt).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	cycleLen := total
	if cycleLen > MaxCycleSeconds {
		cycleLen = MaxCycleSeconds
	}
	position := elapsed % cycleLen

	// Walk the playlist to find which item covers `position`.
	// If cycleLen was capped below total (long playlist case), position
	// still indexes correctly into the same playlist from the start.
	acc := 0
	for _, item := range w.Playlist {
		if position < acc+item.DurationSeconds {
			return item, position - acc
		}
		acc += item.DurationSeconds
	}
	// Fallback (shouldn't normally hit this): last item.
	last := w.Playlist[len(w.Playlist)-1]
	return last, 0
}

// computeDisplay returns what a single window should show right now,
// taking an active sync override into account. If a sync is active and
// has not yet expired, every window shows the synced item instead of its
// own computed position. Once EndsAt has passed, this function naturally
// falls back to the window's normal sequence - no cleanup job needed,
// because the check is a simple time comparison done on every call.
func computeDisplay(w *Window, sync SyncState, now time.Time) CurrentDisplay {
	if sync.Active && now.Before(sync.EndsAt) {
		return CurrentDisplay{
			WindowID:   w.ID,
			WindowName: w.Name,
			Item:       sync.Item,
			IsSynced:   true,
		}
	}
	item, pos := currentItemForWindow(w, now)
	return CurrentDisplay{
		WindowID:        w.ID,
		WindowName:      w.Name,
		Item:            item,
		IsSynced:        false,
		PositionSeconds: pos,
	}
}
