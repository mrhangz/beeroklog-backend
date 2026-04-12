package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/middleware"
	"github.com/mrhangz/beeroklog-backend/internal/model"
)

type ReviewHandler struct {
	db *pgxpool.Pool
}

func NewReview(db *pgxpool.Pool) *ReviewHandler {
	return &ReviewHandler{db: db}
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
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv
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

	h.attachPhotos(r, reviews)

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
	err := h.db.QueryRow(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 WHERE r.id = $1 AND r.user_id = $2`, id, userID,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt,
		&beer.ID, &beer.Name, &beer.Brewery, &beer.Style, &beer.ABV)
	if err != nil {
		respondError(w, http.StatusNotFound, "review not found")
		return
	}
	rev.Beer = &beer

	photos, _ := h.getPhotos(r, rev.ID)
	rev.Photos = photos

	respondJSON(w, http.StatusOK, rev)
}

func (h *ReviewHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req model.CreateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
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
		if req.Beer.Name == "" {
			respondError(w, http.StatusBadRequest, "beer name is required")
			return
		}
		err := tx.QueryRow(ctx,
			`INSERT INTO beers (name, brewery, style, abv, created_by)
			 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			req.Beer.Name, req.Beer.Brewery, req.Beer.Style, req.Beer.ABV, userID,
		).Scan(&beerID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "insert beer failed")
			return
		}
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
		`INSERT INTO reviews (user_id, beer_id, rating, review_text, tasted_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, beer_id, rating, review_text, tasted_at, created_at, updated_at`,
		userID, beerID, req.Rating, req.ReviewText, tastedAt,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt)
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

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(ctx)

	// Verify ownership
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

	if req.PhotoKeys != nil {
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

	var rev model.Review
	_ = h.db.QueryRow(ctx,
		`SELECT id, user_id, beer_id, rating, review_text, tasted_at, created_at, updated_at
		 FROM reviews WHERE id = $1`, id,
	).Scan(&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt)

	respondJSON(w, http.StatusOK, rev)
}

func (h *ReviewHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	id := chi.URLParam(r, "id")

	tag, err := h.db.Exec(r.Context(),
		`DELETE FROM reviews WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil || tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "review not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func scanReviewsWithBeer(rows pgx.Rows) ([]model.Review, error) {
	var reviews []model.Review
	for rows.Next() {
		var rev model.Review
		var beer model.Beer
		if err := rows.Scan(
			&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt,
			&beer.ID, &beer.Name, &beer.Brewery, &beer.Style, &beer.ABV,
		); err != nil {
			return nil, err
		}
		rev.Beer = &beer
		reviews = append(reviews, rev)
	}
	if reviews == nil {
		reviews = []model.Review{}
	}
	return reviews, nil
}

func (h *ReviewHandler) getPhotos(r *http.Request, reviewID string) ([]model.ReviewPhoto, error) {
	rows, err := h.db.Query(r.Context(),
		`SELECT id, review_id, storage_key, sort_order
		 FROM review_photos WHERE review_id = $1 ORDER BY sort_order`, reviewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photos []model.ReviewPhoto
	for rows.Next() {
		var p model.ReviewPhoto
		if err := rows.Scan(&p.ID, &p.ReviewID, &p.StorageKey, &p.SortOrder); err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	if photos == nil {
		photos = []model.ReviewPhoto{}
	}
	return photos, nil
}

func (h *ReviewHandler) attachPhotos(r *http.Request, reviews []model.Review) {
	for i := range reviews {
		photos, _ := h.getPhotos(r, reviews[i].ID)
		reviews[i].Photos = photos
	}
}
