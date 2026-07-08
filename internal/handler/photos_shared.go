package handler

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/model"
)

// photoURLPath is the API path clients fetch a photo from; the photo handler
// redirects it to a presigned S3 URL.
func photoURLPath(storageKey string) string {
	return "/api/photos/" + storageKey
}

// loadPhotos fetches photos for all given reviews in a single query and
// attaches them in sort order.
func loadPhotos(ctx context.Context, db *pgxpool.Pool, reviews []model.Review) error {
	if len(reviews) == 0 {
		return nil
	}

	ids := make([]string, len(reviews))
	byReview := make(map[string]int, len(reviews))
	for i := range reviews {
		ids[i] = reviews[i].ID
		byReview[reviews[i].ID] = i
		reviews[i].Photos = []model.ReviewPhoto{}
	}

	rows, err := db.Query(ctx,
		`SELECT id, review_id, storage_key, sort_order
		 FROM review_photos
		 WHERE review_id = ANY($1::uuid[])
		 ORDER BY review_id, sort_order`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var p model.ReviewPhoto
		if err := rows.Scan(&p.ID, &p.ReviewID, &p.StorageKey, &p.SortOrder); err != nil {
			return err
		}
		p.URL = photoURLPath(p.StorageKey)
		if i, ok := byReview[p.ReviewID]; ok {
			reviews[i].Photos = append(reviews[i].Photos, p)
		}
	}
	return rows.Err()
}
