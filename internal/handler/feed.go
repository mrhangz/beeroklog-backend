package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/model"
)

type FeedHandler struct {
	db *pgxpool.Pool
}

func NewFeed(db *pgxpool.Pool) *FeedHandler {
	return &FeedHandler{db: db}
}

// Latest returns the most recent reviews from all users.
func (h *FeedHandler) Latest(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePage(r)
	sortCol := parseSort(r)
	offset := (page - 1) * perPage

	var totalCount int
	if err := h.db.QueryRow(r.Context(), `SELECT count(*) FROM reviews`).Scan(&totalCount); err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv,
		        u.id, u.display_name, u.avatar_url
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 JOIN users u ON u.id = r.user_id
		 ORDER BY `+sortCol+` DESC
		 LIMIT $1 OFFSET $2`, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	reviews, err := scanFeedReviews(rows)
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

// ByBeer returns all reviews for a specific beer.
func (h *FeedHandler) ByBeer(w http.ResponseWriter, r *http.Request) {
	beerID := chi.URLParam(r, "beerId")
	page, perPage := parsePage(r)
	sortCol := parseSort(r)
	offset := (page - 1) * perPage

	var totalCount int
	if err := h.db.QueryRow(r.Context(),
		`SELECT count(*) FROM reviews WHERE beer_id = $1`, beerID,
	).Scan(&totalCount); err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv,
		        u.id, u.display_name, u.avatar_url
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 JOIN users u ON u.id = r.user_id
		 WHERE r.beer_id = $1
		 ORDER BY `+sortCol+` DESC
		 LIMIT $2 OFFSET $3`, beerID, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	reviews, err := scanFeedReviews(rows)
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

// ByUser returns all reviews from a specific user.
func (h *FeedHandler) ByUser(w http.ResponseWriter, r *http.Request) {
	targetUserID := chi.URLParam(r, "userId")
	page, perPage := parsePage(r)
	sortCol := parseSort(r)
	offset := (page - 1) * perPage

	var totalCount int
	if err := h.db.QueryRow(r.Context(),
		`SELECT count(*) FROM reviews WHERE user_id = $1`, targetUserID,
	).Scan(&totalCount); err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT r.id, r.user_id, r.beer_id, r.rating, r.review_text, r.tasted_at, r.created_at, r.updated_at,
		        b.id, b.name, b.brewery, b.style, b.abv,
		        u.id, u.display_name, u.avatar_url
		 FROM reviews r
		 JOIN beers b ON b.id = r.beer_id
		 JOIN users u ON u.id = r.user_id
		 WHERE r.user_id = $1
		 ORDER BY `+sortCol+` DESC
		 LIMIT $2 OFFSET $3`, targetUserID, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	reviews, err := scanFeedReviews(rows)
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

func scanFeedReviews(rows pgx.Rows) ([]model.Review, error) {
	var reviews []model.Review
	for rows.Next() {
		var rev model.Review
		var beer model.Beer
		var user model.User
		if err := rows.Scan(
			&rev.ID, &rev.UserID, &rev.BeerID, &rev.Rating, &rev.ReviewText, &rev.TastedAt, &rev.CreatedAt, &rev.UpdatedAt,
			&beer.ID, &beer.Name, &beer.Brewery, &beer.Style, &beer.ABV,
			&user.ID, &user.DisplayName, &user.AvatarURL,
		); err != nil {
			return nil, err
		}
		rev.Beer = &beer
		rev.User = &user
		reviews = append(reviews, rev)
	}
	if reviews == nil {
		reviews = []model.Review{}
	}
	return reviews, rows.Err()
}
