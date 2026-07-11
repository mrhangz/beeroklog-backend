package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/middleware"
	"github.com/mrhangz/beeroklog-backend/internal/model"
)

type BeerHandler struct {
	db *pgxpool.Pool
}

func NewBeer(db *pgxpool.Pool) *BeerHandler {
	return &BeerHandler{db: db}
}

func (h *BeerHandler) List(w http.ResponseWriter, r *http.Request) {
	page, perPage := parsePage(r)
	offset := (page - 1) * perPage
	q := r.URL.Query().Get("q")
	pattern := "%" + q + "%"

	var totalCount int
	err := h.db.QueryRow(r.Context(),
		`SELECT count(*) FROM beers
		 WHERE $1 = '' OR lower(name) LIKE lower($2) OR lower(brewery) LIKE lower($2)`, q, pattern,
	).Scan(&totalCount)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT b.id, b.name, b.brewery, b.style, b.abv, b.created_by, b.created_at,`+beerAggregateSelect+`
		 FROM beers b
		 LEFT JOIN reviews r ON r.beer_id = b.id
		 WHERE $1 = '' OR lower(b.name) LIKE lower($2) OR lower(b.brewery) LIKE lower($2)
		 GROUP BY b.id
		 ORDER BY b.name
		 LIMIT $3 OFFSET $4`, q, pattern, perPage, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	beers := []model.Beer{}
	for rows.Next() {
		var b model.Beer
		var avg float64
		var cnt int
		if err := rows.Scan(&b.ID, &b.Name, &b.Brewery, &b.Style, &b.ABV, &b.CreatedBy, &b.CreatedAt, &avg, &cnt); err != nil {
			respondError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		b.AvgRating = &avg
		b.ReviewCount = &cnt
		beers = append(beers, b)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	respondJSON(w, http.StatusOK, model.PaginatedResponse{
		Data:       beers,
		TotalCount: totalCount,
		Page:       page,
		PerPage:    perPage,
	})
}

func (h *BeerHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var b model.Beer
	var avg float64
	var cnt int
	err := h.db.QueryRow(r.Context(),
		`SELECT b.id, b.name, b.brewery, b.style, b.abv, b.created_by, b.created_at,`+beerAggregateSelect+`
		 FROM beers b
		 LEFT JOIN reviews r ON r.beer_id = b.id
		 WHERE b.id = $1
		 GROUP BY b.id`, id,
	).Scan(&b.ID, &b.Name, &b.Brewery, &b.Style, &b.ABV, &b.CreatedBy, &b.CreatedAt, &avg, &cnt)
	if err != nil {
		respondError(w, http.StatusNotFound, "beer not found")
		return
	}
	b.AvgRating = &avg
	b.ReviewCount = &cnt

	respondJSON(w, http.StatusOK, b)
}

func (h *BeerHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req model.CreateBeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	ctx := r.Context()
	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "tx begin failed")
		return
	}
	defer tx.Rollback(ctx)

	// Reuse an existing catalog row when name+brewery already match
	// (case-insensitive). Prevents new duplicates from direct creates.
	beerID, err := findOrCreateBeer(ctx, tx, userID, &req)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "insert beer failed")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	b, err := loadBeerWithAggregates(ctx, h.db, beerID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}
	respondJSON(w, http.StatusCreated, b)
}
