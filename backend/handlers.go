package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type API struct {
	store *Store
}

func NewAPI(store *Store) *API {
	return &API{store: store}
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("error encoding response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// enableCORS allows the deployed React frontend (on a different origin)
// to call this API. Locked down to specific origins via env var in
// production - see main.go.
func enableCORS(allowedOrigin string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// GET /api/windows - list every window and its full playlist.
func (a *API) handleListWindows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, a.store.AllWindows())
}

// GET /api/state - for every window, what it should be displaying right now.
// This is the endpoint the frontend polls continuously.
func (a *API) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	now := time.Now()
	syncState := a.store.GetSyncState()
	windows := a.store.AllWindows()

	displays := make([]CurrentDisplay, 0, len(windows))
	for _, win := range windows {
		displays = append(displays, computeDisplay(win, syncState, now))
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"windows":    displays,
		"sync":       syncState,
		"serverTime": now,
	})
}

// addMediaRequest is the JSON body for POST /api/windows/{id}/media
type addMediaRequest struct {
	Type            MediaType `json:"type"`
	URL             string    `json:"url"`
	DurationSeconds int       `json:"durationSeconds"`
}

// POST /api/windows/{id}/media - dynamically add an item to a window's playlist.
func (a *API) handleAddMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	windowID := r.PathValue("id")

	var req addMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Type != MediaImage && req.Type != MediaVideo && req.Type != MediaBlank {
		writeError(w, http.StatusBadRequest, "type must be image, video, or blank")
		return
	}
	if req.Type != MediaBlank && req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required for image/video items")
		return
	}
	if req.DurationSeconds <= 0 {
		writeError(w, http.StatusBadRequest, "durationSeconds must be > 0")
		return
	}

	item := MediaItem{
		ID:              fmt.Sprintf("m-%d", time.Now().UnixNano()),
		Type:            req.Type,
		URL:             req.URL,
		DurationSeconds: req.DurationSeconds,
	}

	updated, err := a.store.AddMediaToWindow(windowID, item)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// syncRequest is the JSON body for POST /api/sync
type syncRequest struct {
	MediaID         string `json:"mediaId"`
	DurationSeconds int    `json:"durationSeconds"`
}

// POST /api/sync - trigger every window to show one media item together.
//
// Implementation note: the client sends the mediaId of an item that
// already exists in at least one window's playlist; we look up its full
// definition (type/url/duration-to-display) so every window renders an
// identical item regardless of where it originally lived.
func (a *API) handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.MediaID == "" {
		writeError(w, http.StatusBadRequest, "mediaId is required")
		return
	}
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 10 // sensible default sync duration
	}

	var found *MediaItem
	for _, win := range a.store.AllWindows() {
		for _, item := range win.Playlist {
			if item.ID == req.MediaID {
				found = &item
				break
			}
		}
		if found != nil {
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "no media item with that id exists in any window")
		return
	}

	now := time.Now()
	state := SyncState{
		Active:    true,
		MediaID:   found.ID,
		Item:      *found,
		StartedAt: now,
		EndsAt:    now.Add(time.Duration(req.DurationSeconds) * time.Second),
	}
	if err := a.store.SetSyncState(state); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save sync state")
		return
	}
	writeJSON(w, http.StatusOK, state)
}

// GET /api/sync - current sync status (mainly useful for debugging/admin UI).
func (a *API) handleGetSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, a.store.GetSyncState())
}
