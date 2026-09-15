package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	dataFile := getEnv("DATA_FILE", "./data/store.db")
	port := getEnv("PORT", "8080")
	
	frontendOrigin := getEnv("FRONTEND_ORIGIN", "*")
	
	publicURL := getEnv("BACKEND_PUBLIC_URL", "http://localhost:"+port)

	
	uploadsDir := filepath.Join(filepath.Dir(dataFile), "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		log.Fatalf("failed to create uploads dir: %v", err)
	}

	store, err := NewStore(dataFile)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}

	// Seed on first run only - if the store file didn't exist yet,
	// NewStore will have left Windows empty.
	if len(store.AllWindows()) == 0 {
		log.Println("no existing data found, seeding example windows...")
		for _, w := range seedWindows() {
			if err := store.UpsertWindow(w); err != nil {
				log.Fatalf("failed to seed window %s: %v", w.ID, err)
			}
		}
	}

	api := NewAPI(store)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/windows", enableCORS(frontendOrigin, api.handleListWindows))
	mux.HandleFunc("/api/windows/{id}/media", enableCORS(frontendOrigin, api.handleAddMedia))
	mux.HandleFunc("/api/state", enableCORS(frontendOrigin, api.handleState))
	mux.HandleFunc("/api/sync", enableCORS(frontendOrigin, combineGetPost(api.handleGetSync, api.handleSync)))
	mux.HandleFunc("/api/upload", enableCORS(frontendOrigin, api.handleUpload(uploadsDir, publicURL)))

	// Serves uploaded files back out, e.g. GET /uploads/u-12345.jpg.
	fileServer := http.StripPrefix("/uploads/", http.FileServer(http.Dir(uploadsDir)))
	mux.HandleFunc("/uploads/", enableCORS(frontendOrigin, fileServer.ServeHTTP))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("media-sequencer backend listening on :%s (data file: %s)", port, dataFile)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// combineGetPost routes GET and POST on the same path to two different
// handlers, since net/http's ServeMux (pre-1.22 method matching aside)
// is simplest to reason about with one explicit dispatcher per path.
func combineGetPost(getHandler, postHandler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getHandler(w, r)
		case http.MethodPost:
			postHandler(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
