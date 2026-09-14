package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestMux builds the same route table as main.go, but against a fresh
// in-memory SQLite store, so PathValue-based routing (e.g. {id} in
// /api/windows/{id}/media) behaves exactly as it does in production.
func newTestMux(t *testing.T) (*http.ServeMux, *Store) {
	t.Helper()
	store, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory store: %v", err)
	}
	if err := store.UpsertWindow(&Window{
		ID:   "window-1",
		Name: "Test Window",
		Playlist: []MediaItem{
			{ID: "m-seed-1", Type: MediaImage, URL: "https://example.com/1.jpg", DurationSeconds: 8},
		},
	}); err != nil {
		t.Fatalf("failed to seed window: %v", err)
	}

	api := NewAPI(store)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/windows", api.handleListWindows)
	mux.HandleFunc("/api/windows/{id}/media", api.handleAddMedia)
	mux.HandleFunc("/api/state", api.handleState)
	mux.HandleFunc("/api/sync", combineGetPost(api.handleGetSync, api.handleSync))
	return mux, store
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// ---------- GET /api/windows ----------

func TestHandleListWindows_ReturnsSeededWindow(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodGet, "/api/windows", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var windows []Window
	if err := json.Unmarshal(rec.Body.Bytes(), &windows); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(windows) != 1 || windows[0].ID != "window-1" {
		t.Errorf("expected exactly the seeded window-1, got %+v", windows)
	}
}

// ---------- POST /api/windows/{id}/media ----------

func TestHandleAddMedia_Success(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/windows/window-1/media", map[string]interface{}{
		"type": "video", "url": "https://example.com/clip.mp4", "durationSeconds": 12,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated Window
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(updated.Playlist) != 2 {
		t.Fatalf("expected playlist to grow to 2 items, got %d", len(updated.Playlist))
	}
	last := updated.Playlist[len(updated.Playlist)-1]
	if last.Type != MediaVideo || last.URL != "https://example.com/clip.mp4" || last.DurationSeconds != 12 {
		t.Errorf("new item fields don't match request: %+v", last)
	}
}

func TestHandleAddMedia_UnknownWindow(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/windows/does-not-exist/media", map[string]interface{}{
		"type": "image", "url": "https://example.com/x.jpg", "durationSeconds": 5,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown window, got %d", rec.Code)
	}
}

func TestHandleAddMedia_InvalidType(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/windows/window-1/media", map[string]interface{}{
		"type": "gif", "url": "https://example.com/x.gif", "durationSeconds": 5,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid type, got %d", rec.Code)
	}
}

func TestHandleAddMedia_MissingURLForImage(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/windows/window-1/media", map[string]interface{}{
		"type": "image", "durationSeconds": 5,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when url missing for image type, got %d", rec.Code)
	}
}

func TestHandleAddMedia_BlankDoesNotRequireURL(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/windows/window-1/media", map[string]interface{}{
		"type": "blank", "durationSeconds": 3,
	})
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for blank item without url, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAddMedia_ZeroOrNegativeDuration(t *testing.T) {
	mux, _ := newTestMux(t)
	for _, dur := range []int{0, -5} {
		rec := doJSON(t, mux, http.MethodPost, "/api/windows/window-1/media", map[string]interface{}{
			"type": "image", "url": "https://example.com/x.jpg", "durationSeconds": dur,
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("durationSeconds=%d: expected 400, got %d", dur, rec.Code)
		}
	}
}

func TestHandleAddMedia_WrongMethod(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodGet, "/api/windows/window-1/media", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET on add-media endpoint, got %d", rec.Code)
	}
}

// ---------- POST /api/sync ----------

func TestHandleSync_Success(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/sync", map[string]interface{}{
		"mediaId": "m-seed-1", "durationSeconds": 15,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var state SyncState
	if err := json.Unmarshal(rec.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !state.Active || state.MediaID != "m-seed-1" {
		t.Errorf("expected active sync on m-seed-1, got %+v", state)
	}
}

func TestHandleSync_UnknownMediaID(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/sync", map[string]interface{}{
		"mediaId": "does-not-exist", "durationSeconds": 10,
	})
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown mediaId, got %d", rec.Code)
	}
}

func TestHandleSync_MissingMediaID(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/sync", map[string]interface{}{
		"durationSeconds": 10,
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when mediaId missing, got %d", rec.Code)
	}
}

func TestHandleSync_DefaultsDurationWhenOmitted(t *testing.T) {
	mux, _ := newTestMux(t)
	rec := doJSON(t, mux, http.MethodPost, "/api/sync", map[string]interface{}{
		"mediaId": "m-seed-1",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var state SyncState
	_ = json.Unmarshal(rec.Body.Bytes(), &state)
	gotDuration := int(state.EndsAt.Sub(state.StartedAt).Seconds())
	if gotDuration != 10 {
		t.Errorf("expected default 10s sync duration per README, got %ds", gotDuration)
	}
}

// ---------- GET /api/state ----------

func TestHandleState_ReflectsSyncAcrossWindows(t *testing.T) {
	mux, store := newTestMux(t)
	// Add a second window so we can confirm sync affects *every* window.
	if err := store.UpsertWindow(&Window{
		ID:   "window-2",
		Name: "Second Window",
		Playlist: []MediaItem{
			{ID: "m-other", Type: MediaImage, URL: "https://example.com/other.jpg", DurationSeconds: 5},
		},
	}); err != nil {
		t.Fatalf("seed window-2: %v", err)
	}

	// Trigger sync on the item that lives in window-1's playlist.
	syncRec := doJSON(t, mux, http.MethodPost, "/api/sync", map[string]interface{}{
		"mediaId": "m-seed-1", "durationSeconds": 30,
	})
	if syncRec.Code != http.StatusOK {
		t.Fatalf("sync trigger failed: %d %s", syncRec.Code, syncRec.Body.String())
	}

	stateRec := doJSON(t, mux, http.MethodGet, "/api/state", nil)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", stateRec.Code, stateRec.Body.String())
	}
	var resp struct {
		Windows []CurrentDisplay `json:"windows"`
	}
	if err := json.Unmarshal(stateRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Windows) != 2 {
		t.Fatalf("expected 2 windows in state, got %d", len(resp.Windows))
	}
	for _, d := range resp.Windows {
		if !d.IsSynced || d.Item.ID != "m-seed-1" {
			t.Errorf("expected window %s to show synced item m-seed-1, got isSynced=%v item=%s",
				d.WindowID, d.IsSynced, d.Item.ID)
		}
	}
}
