package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/middleware"
	"github.com/mrhangz/beeroklog-backend/internal/model"
	"github.com/mrhangz/beeroklog-backend/internal/storage"
)

type ReviewHandler struct {
	db *pgxpool.Pool
	s3 *storage.S3
}

func NewReview(db *pgxpool.Pool, s3 *storage.S3) *ReviewHandler {
	return &ReviewHandler{db: db, s3: s3}
}

func (h *ReviewHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	page, perPage := parsePage(r)
	offset := (page - 1) * perPage

	var totalCount int
	if err := h.db.QueryRow(r.Context(),
		`SELECT count(*) FROM reviews WHERE user_id = $1`, userID,
	).Scan(&totalCount); err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.serving_size_ml, r.serving_count, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv, b.image_storage_key
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 WHERE r.user_id = $1
		 ORDER BY r.created_at DESC
		 LIMIT $2 OFFSET $3`, userID, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	reviews, err := scanReviewsWithBeer(rows)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	if err := loadPhotos(r.Context(), h.db, reviews); err != nil {
		respondError(w, http.StatusInternalServerError, "load photos failed")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       reviews,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
	})
}

func (h *ReviewHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	var rev model.Review
	var beer model.Beer
	var imageKey *string
	err := h.db.QueryRow(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.serving_size_ml, r.serving_count, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv, b.image_storage_key
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 WHERE r.id = $1 AND r.user_id = $2`, id, userID,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.ServingSizeML, &rev.ServingCount, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt,
		&beer.ID, &beer.Name, &beer.Brewery, &beer.Style, &beer.ABV, &imageKey)
	if err != nil {
		respondError(w, http.StatusNotFound, "review not found")
		return
	}
	if imageKey != nil {
		beer.ImageStorageKey = *imageKey
	}
	attachBeerImage(&beer)
	rev.Beer = &beer

	reviews := []model.Review{rev}
	if err := loadPhotos(r.Context(), h.db, reviews); err != nil {
		respondError(w, http.StatusInternalServerError, "load photos failed")
		return
	}

	respondJSON(w, http.StatusOK, reviews[0])
}

func (h *ReviewHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req model.CreateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Rating < 0 || req.Rating > 5 {
		respondError(w, http.StatusBadRequest, "rating must be between 0 and 5")
		return
	}
	size, count, err := normalizeServing(req.ServingSizeML, req.ServingCount)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(ctx)

	beerID := req.BeerID
	if beerID == "" && req.Beer != nil {
		if strings.TrimSpace(req.Beer.Name) == "" {
			respondError(w, http.StatusBadRequest, "beer name is required")
			return
		}
		id, err := findOrCreateBeer(ctx, tx, userID, req.Beer)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "insert beer failed")
			return
		}
		beerID = id
	}

	if beerID == "" {
		respondError(w, http.StatusBadRequest, "beer_id or beer object is required")
		return
	}

	tastedAt := time.Now()
	if req.TastedAt != nil {
		tastedAt = *req.TastedAt
	}

	var rev model.Review
	err = tx.QueryRow(ctx,
		`INSERT INTO reviews (user_id, beer_id, rating, review_text, serving_size_ml, serving_count, tasted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id, user_id, beer_id, rating, review_text, serving_size_ml, serving_count, tasted_at, created_at, updated_at`,
		userID, beerID, req.Rating, req.ReviewText, size, count, tastedAt,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.ServingSizeML, &rev.ServingCount, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "insert review failed")
		return
	}

	for i, key := range req.PhotoKeys {
		_, err := tx.Exec(ctx,
			`INSERT INTO review_photos (review_id, storage_key, sort_order) VALUES ($1, $2, $3)`,
			rev.ID, key, i)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "insert photo failed")
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	if err := attachBeerAndPhotos(ctx, h.db, &rev); err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}

	respondJSON(w, http.StatusCreated, rev)
}

func (h *ReviewHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	var req model.UpdateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Rating != nil && (*req.Rating < 0 || *req.Rating > 5) {
		respondError(w, http.StatusBadRequest, "rating must be between 0 and 5")
		return
	}
	// Beer catalog fields on update are ignored: beers are global master data.
	// Clients should not mutate shared beer rows via pour/review edits.

	var servingSize, servingCount *int
	if req.ClearServing {
		// both stay nil → clear columns
	} else if req.ServingSizeML != nil || req.ServingCount != nil {
		var err error
		servingSize, servingCount, err = normalizeServing(req.ServingSizeML, req.ServingCount)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(ctx)

	// Verify ownership.
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM reviews WHERE id = $1 AND user_id = $2)`, id, userID,
	).Scan(&exists); err != nil || !exists {
		respondError(w, http.StatusNotFound, "review not found")
		return
	}

	if req.Rating != nil {
		if _, err := tx.Exec(ctx, `UPDATE reviews SET rating = $1, updated_at = now() WHERE id = $2`, *req.Rating, id); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if req.ReviewText != nil {
		if _, err := tx.Exec(ctx, `UPDATE reviews SET review_text = $1, updated_at = now() WHERE id = $2`, *req.ReviewText, id); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if req.TastedAt != nil {
		if _, err := tx.Exec(ctx, `UPDATE reviews SET tasted_at = $1, updated_at = now() WHERE id = $2`, *req.TastedAt, id); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if req.ClearServing || req.ServingSizeML != nil || req.ServingCount != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE reviews SET serving_size_ml = $1, serving_count = $2, updated_at = now() WHERE id = $3`,
			servingSize, servingCount, id,
		); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}

	var removedKeys []string
	if req.PhotoKeys != nil {
		oldKeys, err := storageKeysForReview(ctx, tx, id)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "load photos failed")
			return
		}
		kept := make(map[string]bool, len(req.PhotoKeys))
		for _, key := range req.PhotoKeys {
			kept[key] = true
		}
		for _, key := range oldKeys {
			if !kept[key] {
				removedKeys = append(removedKeys, key)
			}
		}

		if _, err := tx.Exec(ctx, `DELETE FROM review_photos WHERE review_id = $1`, id); err != nil {
			respondError(w, http.StatusInternalServerError, "delete photos failed")
			return
		}
		for i, key := range req.PhotoKeys {
			if _, err := tx.Exec(ctx,
				`INSERT INTO review_photos (review_id, storage_key, sort_order) VALUES ($1, $2, $3)`,
				id, key, i); err != nil {
				respondError(w, http.StatusInternalServerError, "insert photo failed")
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	h.deleteFromS3(removedKeys)

	var rev model.Review
	err = h.db.QueryRow(ctx,
		`SELECT id, user_id, beer_id, rating, review_text, serving_size_ml, serving_count, tasted_at, created_at, updated_at
		 FROM reviews WHERE id = $1`, id,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.ServingSizeML, &rev.ServingCount, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}

	if err := attachBeerAndPhotos(ctx, h.db, &rev); err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}

	respondJSON(w, http.StatusOK, rev)
}

func (h *ReviewHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	keys, err := storageKeysForReview(r.Context(), h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	tag, err := h.db.Exec(r.Context(),
		`DELETE FROM reviews WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil || tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "review not found")
		return
	}

	h.deleteFromS3(keys)

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

// deleteFromS3 removes photo objects best-effort; DB rows are already gone,
// so a failure only leaves an orphaned object behind.
func (h *ReviewHandler) deleteFromS3(keys []string) {
	if h.s3 == nil {
		return
	}
	for _, key := range keys {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := h.s3.Delete(ctx, key); err != nil {
			log.Printf("delete s3 object %s: %v", key, err)
		}
		cancel()
	}
}

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func storageKeysForReview(ctx context.Context, db querier, reviewID string) ([]string, error) {
	rows, err := db.Query(ctx,
		`SELECT storage_key FROM review_photos WHERE review_id = $1`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func scanReviewsWithBeer(rows pgx.Rows) ([]model.Review, error) {
	var reviews []model.Review
	for rows.Next() {
		var rev model.Review
		var beer model.Beer
		var imageKey *string
		if err := rows.Scan(
			&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.ServingSizeML, &rev.ServingCount, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt,
			&beer.ID, &beer.Name, &beer.Brewery, &beer.Style, &beer.ABV, &imageKey,
		); err != nil {
			return nil, err
		}
		if imageKey != nil {
			beer.ImageStorageKey = *imageKey
		}
		attachBeerImage(&beer)
		rev.Beer = &beer
		reviews = append(reviews, rev)
	}
	if reviews == nil {
		reviews = []model.Review{}
	}
	return reviews, rows.Err()
}
