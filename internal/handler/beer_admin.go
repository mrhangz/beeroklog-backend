package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/model"
)

// Update edits beer master data. Admin only.
func (h *BeerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req model.UpdateBeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == nil && req.Brewery == nil && req.Style == nil && req.ABV == nil && req.ImageStorageKey == nil {
		respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	ctx := r.Context()
	tag, err := h.db.Exec(ctx,
		`UPDATE beers SET
			name = COALESCE($2, name),
			brewery = COALESCE($3, brewery),
			style = COALESCE($4, style),
			abv = CASE WHEN $5::boolean THEN $6 ELSE abv END,
			image_storage_key = CASE
				WHEN $7::boolean THEN NULLIF(btrim($8), '')
				ELSE image_storage_key
			END
		 WHERE id = $1`,
		id,
		nullIfAbsentString(req.Name),
		nullIfAbsentString(req.Brewery),
		nullIfAbsentString(req.Style),
		req.ABV != nil,
		req.ABV,
		req.ImageStorageKey != nil,
		derefString(req.ImageStorageKey),
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "beer not found")
		return
	}

	b, err := loadBeerWithAggregates(ctx, h.db, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}
	respondJSON(w, http.StatusOK, b)
}

// ListDuplicates groups catalog rows that share the same normalized name+brewery.
func (h *BeerHandler) ListDuplicates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.Query(ctx,
		`SELECT b.id, b.name, b.brewery, b.style, b.abv, b.created_by, b.created_at, b.image_storage_key,`+beerAggregateSelect+`
		 FROM beers b
		 LEFT JOIN reviews r ON r.beer_id = b.id
		 WHERE (lower(b.name), lower(coalesce(b.brewery, ''))) IN (
		   SELECT lower(name), lower(coalesce(brewery, ''))
		   FROM beers
		   GROUP BY lower(name), lower(coalesce(brewery, ''))
		   HAVING count(*) > 1
		 )
		 GROUP BY b.id
		 ORDER BY lower(b.name), lower(coalesce(b.brewery, '')), b.created_at`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	groups := []model.BeerDuplicateGroup{}
	index := map[string]int{}
	for rows.Next() {
		b, err := scanBeerWithAggregates(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		key := strings.ToLower(b.Name) + "\x00" + strings.ToLower(b.Brewery)
		if i, ok := index[key]; ok {
			groups[i].Beers = append(groups[i].Beers, *b)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, model.BeerDuplicateGroup{
			Name:    b.Name,
			Brewery: b.Brewery,
			Beers:   []model.Beer{*b},
		})
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	respondJSON(w, http.StatusOK, groups)
}

// Merge combines duplicate beers into one survivor. Admin only.
func (h *BeerHandler) Merge(w http.ResponseWriter, r *http.Request) {
	var req model.MergeBeersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.KeepID == "" || len(req.MergeIDs) == 0 {
		respondError(w, http.StatusBadRequest, "keep_id and merge_ids are required")
		return
	}

	mergeIDs := uniqueIDs(req.MergeIDs)
	for _, id := range mergeIDs {
		if id == req.KeepID {
			respondError(w, http.StatusBadRequest, "merge_ids must not include keep_id")
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

	moved, err := mergeBeersTx(ctx, tx, req.KeepID, mergeIDs)
	if err != nil {
		if err == pgx.ErrNoRows {
			respondError(w, http.StatusNotFound, "beer not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "merge failed")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, "commit failed")
		return
	}

	b, err := loadBeerWithAggregates(ctx, h.db, req.KeepID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "reload failed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"beer":          b,
		"reviews_moved": moved,
		"beers_removed": len(mergeIDs),
	})
}

// DedupeExact auto-merges every exact name+brewery duplicate group, keeping
// the beer with the most rated reviews (ties → oldest created_at).
func (h *BeerHandler) DedupeExact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.Query(ctx,
		`SELECT lower(name), lower(coalesce(brewery, ''))
		 FROM beers
		 GROUP BY lower(name), lower(coalesce(brewery, ''))
		 HAVING count(*) > 1`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type key struct{ name, brewery string }
	var keys []key
	for rows.Next() {
		var k key
		if err := rows.Scan(&k.name, &k.brewery); err != nil {
			respondError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "scan failed")
		return
	}

	result := model.DedupeExactResult{}
	for _, k := range keys {
		tx, err := h.db.Begin(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "tx begin failed")
			return
		}

		idRows, err := tx.Query(ctx,
			`SELECT b.id
			 FROM beers b
			 LEFT JOIN reviews r ON r.beer_id = b.id AND r.rating > 0
			 WHERE lower(b.name) = $1 AND lower(coalesce(b.brewery, '')) = $2
			 GROUP BY b.id
			 ORDER BY count(r.id) DESC, b.created_at ASC`,
			k.name, k.brewery,
		)
		if err != nil {
			tx.Rollback(ctx)
			respondError(w, http.StatusInternalServerError, "query failed")
			return
		}
		var ids []string
		for idRows.Next() {
			var id string
			if err := idRows.Scan(&id); err != nil {
				idRows.Close()
				tx.Rollback(ctx)
				respondError(w, http.StatusInternalServerError, "scan failed")
				return
			}
			ids = append(ids, id)
		}
		idRows.Close()
		if len(ids) < 2 {
			tx.Rollback(ctx)
			continue
		}

		moved, err := mergeBeersTx(ctx, tx, ids[0], ids[1:])
		if err != nil {
			tx.Rollback(ctx)
			respondError(w, http.StatusInternalServerError, "merge failed")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			respondError(w, http.StatusInternalServerError, "commit failed")
			return
		}
		result.GroupsMerged++
		result.BeersRemoved += len(ids) - 1
		result.ReviewsMoved += moved
	}

	respondJSON(w, http.StatusOK, result)
}

func mergeBeersTx(ctx context.Context, tx pgx.Tx, keepID string, mergeIDs []string) (int, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM beers WHERE id = $1)`, keepID).Scan(&exists); err != nil {
		return 0, err
	}
	if !exists {
		return 0, pgx.ErrNoRows
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM beers WHERE id = ANY($1::uuid[])`, mergeIDs,
	).Scan(&count); err != nil {
		return 0, err
	}
	if count != len(mergeIDs) {
		return 0, pgx.ErrNoRows
	}

	for _, id := range mergeIDs {
		if _, err := tx.Exec(ctx,
			`UPDATE beers AS keep
			 SET brewery = CASE WHEN btrim(keep.brewery) = '' THEN src.brewery ELSE keep.brewery END,
			     style = CASE WHEN btrim(keep.style) = '' THEN src.style ELSE keep.style END,
			     abv = COALESCE(keep.abv, src.abv),
			     image_storage_key = CASE
			       WHEN keep.image_storage_key IS NULL OR btrim(keep.image_storage_key) = ''
			       THEN src.image_storage_key
			       ELSE keep.image_storage_key
			     END
			 FROM beers AS src
			 WHERE keep.id = $1 AND src.id = $2`,
			keepID, id,
		); err != nil {
			return 0, err
		}
	}

	tag, err := tx.Exec(ctx,
		`UPDATE reviews SET beer_id = $1 WHERE beer_id = ANY($2::uuid[])`,
		keepID, mergeIDs,
	)
	if err != nil {
		return 0, err
	}
	moved := int(tag.RowsAffected())

	if _, err := tx.Exec(ctx, `DELETE FROM beers WHERE id = ANY($1::uuid[])`, mergeIDs); err != nil {
		return 0, err
	}
	return moved, nil
}

func loadBeerWithAggregates(ctx context.Context, db *pgxpool.Pool, id string) (*model.Beer, error) {
	row := db.QueryRow(ctx,
		`SELECT b.id, b.name, b.brewery, b.style, b.abv, b.created_by, b.created_at, b.image_storage_key,`+beerAggregateSelect+`
		 FROM beers b
		 LEFT JOIN reviews r ON r.beer_id = b.id
		 WHERE b.id = $1
		 GROUP BY b.id`, id,
	)
	return scanBeerWithAggregates(row)
}

type beerAggregateScanner interface {
	Scan(dest ...any) error
}

func scanBeerWithAggregates(row beerAggregateScanner) (*model.Beer, error) {
	var b model.Beer
	var avg float64
	var cnt int
	var imageKey *string
	if err := row.Scan(&b.ID, &b.Name, &b.Brewery, &b.Style, &b.ABV, &b.CreatedBy, &b.CreatedAt, &imageKey, &avg, &cnt); err != nil {
		return nil, err
	}
	if imageKey != nil {
		b.ImageStorageKey = *imageKey
	}
	b.AvgRating = &avg
	b.ReviewCount = &cnt
	attachBeerImage(&b)
	return &b, nil
}

func nullIfAbsentString(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	return &trimmed
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
