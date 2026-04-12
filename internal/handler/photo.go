package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/storage"
)

const maxUploadSize = 10 << 20 // 10 MB

type PhotoHandler struct {
	db *pgxpool.Pool
	s3 *storage.S3
}

func NewPhoto(db *pgxpool.Pool, s3 *storage.S3) *PhotoHandler {
	return &PhotoHandler{db: db, s3: s3}
}

// Upload handles multipart image upload and stores it in S3.
func (h *PhotoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		respondError(w, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing photo field")
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		contentType = "image/jpeg"
	}

	ext := ".jpg"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/heic", "image/heif":
		ext = ".heic"
	}

	key := fmt.Sprintf("photos/%s%s", uuid.New().String(), ext)

	if err := h.s3.Upload(r.Context(), key, file, contentType); err != nil {
		respondError(w, http.StatusInternalServerError, "upload failed")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"storage_key": key,
	})
}

// Get redirects to a presigned S3 URL for the photo.
func (h *PhotoHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	// support nested paths like photos/uuid.jpg
	if rest := chi.URLParam(r, "*"); rest != "" {
		key = key + "/" + rest
	}

	url, err := h.s3.PresignedURL(r.Context(), key, 15*time.Minute)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to generate url")
		return
	}

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}
