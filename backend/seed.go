package main

import "time"

// seedWindows returns the example windows used to populate a fresh store.
// Image/video URLs point at freely-usable placeholder media so the app is
// visibly working immediately after deployment, before a real client
// replaces them with their own assets via the "add media" feature.
func seedWindows() []*Window {
	now := time.Now()
	return []*Window{
		{
			ID:        "window-1",
			Name:      "Window A - Entrance Display",
			CreatedAt: now,
			Playlist: []MediaItem{
				{ID: "m-seed-1", Type: MediaImage, URL: "https://picsum.photos/seed/entrance1/800/450", DurationSeconds: 8},
				{ID: "m-seed-2", Type: MediaVideo, URL: "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4", DurationSeconds: 12},
				{ID: "m-seed-3", Type: MediaImage, URL: "https://picsum.photos/seed/entrance2/800/450", DurationSeconds: 8},
				{ID: "m-seed-4", Type: MediaBlank, DurationSeconds: 4},
			},
		},
		{
			ID:        "window-2",
			Name:      "Window B - Aisle 3 Display",
			CreatedAt: now,
			Playlist: []MediaItem{
				{ID: "m-seed-5", Type: MediaImage, URL: "https://picsum.photos/seed/aisle1/800/450", DurationSeconds: 6},
				{ID: "m-seed-6", Type: MediaImage, URL: "https://picsum.photos/seed/aisle2/800/450", DurationSeconds: 6},
				{ID: "m-seed-7", Type: MediaVideo, URL: "https://interactive-examples.mdn.mozilla.net/media/cc0-videos/flower.mp4", DurationSeconds: 12},
			},
		},
		{
			ID:        "window-3",
			Name:      "Window C - Checkout Display",
			CreatedAt: now,
			Playlist: []MediaItem{
				{ID: "m-seed-8", Type: MediaImage, URL: "https://picsum.photos/seed/checkout1/800/450", DurationSeconds: 10},
				{ID: "m-seed-9", Type: MediaBlank, DurationSeconds: 3},
				{ID: "m-seed-10", Type: MediaImage, URL: "https://picsum.photos/seed/checkout2/800/450", DurationSeconds: 10},
			},
		},
	}
}
