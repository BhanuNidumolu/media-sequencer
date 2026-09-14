package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxUploadSize = 20 << 20 // 20MB

// handleUpload accepts a multipart/form-data file upload (field name
// "file"), saves it under uploadsDir, and returns the URL the frontend
// then passes as the "url" field to POST /api/windows/{id}/media - so an
// uploaded file and a pasted link end up going through the exact same
// add-media code path once we have a URL for either.
func (a *API) handleUpload(uploadsDir, publicBaseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			writeError(w, http.StatusBadRequest, "file too large or invalid form (max 20MB)")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing file field")
			return
		}
		defer file.Close()

		ext := strings.ToLower(filepath.Ext(header.Filename))
		allowedExt := map[string]string{
			".jpg": "image", ".jpeg": "image", ".png": "image", ".gif": "image", ".webp": "image",
			".mp4": "video", ".webm": "video", ".mov": "video",
		}
		mediaType, ok := allowedExt[ext]
		if !ok {
			writeError(w, http.StatusBadRequest, "unsupported file type: "+ext)
			return
		}

		filename := fmt.Sprintf("u-%d%s", time.Now().UnixNano(), ext)
		destPath := filepath.Join(uploadsDir, filename)

		dest, err := os.Create(destPath)
		if err != nil {
			log.Printf("upload create error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}
		defer dest.Close()

		if _, err := io.Copy(dest, file); err != nil {
			log.Printf("upload write error: %v", err)
			writeError(w, http.StatusInternalServerError, "failed to save file")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{
			"url":  publicBaseURL + "/uploads/" + filename,
			"type": mediaType,
		})
	}
}
