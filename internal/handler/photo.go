package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/smithy-go"
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

// Get streams the photo from S3 through the API.
//
// We proxy bytes instead of redirecting to a presigned MinIO URL so browsers
// and mobile clients never need to reach the internal Docker hostname
// (e.g. http://minio:9000), which is unreachable outside the compose network.
func (h *PhotoHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	// support nested paths like photos/uuid.jpg
	if rest := chi.URLParam(r, "*"); rest != "" {
		key = key + "/" + rest
	}

	obj, err := h.s3.Get(r.Context(), key)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound") {
			respondError(w, http.StatusNotFound, "photo not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch photo")
		return
	}
	defer obj.Body.Close()

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if obj.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", obj.Size))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj.Body)
}
