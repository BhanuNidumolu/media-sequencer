package main

import "time"

// MediaType enumerates the kinds of media a playlist item can be.
type MediaType string

const (
	MediaImage MediaType = "image"
	MediaVideo MediaType = "video"
	MediaBlank MediaType = "blank"
)

// MediaItem is a single entry in a window's playlist.
// DurationSeconds is how long this item plays before the next one starts.
type MediaItem struct {
	ID              string    `json:"id"`
	Type            MediaType `json:"type"`
	URL             string    `json:"url,omitempty"` // empty for "blank" items
	DurationSeconds int       `json:"durationSeconds"`
}

// Window represents one display/output that plays its own looping playlist.
// CreatedAt is used as the fixed time anchor from which playback position
// is calculated (see sync.go: currentItemForWindow).
type Window struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Playlist  []MediaItem `json:"playlist"`
	CreatedAt time.Time   `json:"createdAt"`
}

// MaxCycleSeconds is the "treat the total play size as 5 hours" rule from
// the spec. A window's playlist is understood to loop within this ceiling.
// In practice a playlist's own total duration will be far smaller than this,
// so the playlist naturally repeats many times inside one 5-hour cycle -
// see README "Assumptions" for the full reasoning.
const MaxCycleSeconds = 5 * 60 * 60 // 18000 seconds

// SyncState describes an in-progress "show this item on every window" event.
// It is timestamp-driven (StartedAt/EndsAt) rather than a boolean flag so
// that the correct state can always be re-derived from wall-clock time,
// even after a server restart.
type SyncState struct {
	Active    bool      `json:"active"`
	MediaID   string    `json:"mediaId,omitempty"`
	Item      MediaItem `json:"item,omitempty"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	EndsAt    time.Time `json:"endsAt,omitempty"`
}

// CurrentDisplay is what one window should be showing at this instant -
// this is the shape returned by GET /api/state for each window.
type CurrentDisplay struct {
	WindowID   string    `json:"windowId"`
	WindowName string    `json:"windowName"`
	Item       MediaItem `json:"item"`
	IsSynced   bool      `json:"isSynced"`
	// PositionSeconds is how far into the current item playback is,
	// useful for the frontend to resume video playback mid-item on refresh.
	PositionSeconds int `json:"positionSeconds"`
}
