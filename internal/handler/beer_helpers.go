package handler

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/model"
)

// SQL fragments for beer aggregates. Only rated pours (rating > 0) affect
// avg_rating and review_count so log-only entries don't dilute the catalog.
const beerAggregateSelect = `
		        COALESCE(avg(r.rating) FILTER (WHERE r.rating > 0), 0),
		        count(r.id) FILTER (WHERE r.rating > 0)`

// publishedReviewSQL is true for rows that should appear on public feeds /
// beer detail lists: either rated or with written notes. Log-only pours stay
// private to the owner's journal.
const publishedReviewSQL = `(r.rating > 0 OR btrim(coalesce(r.review_text, '')) <> '')`

// findOrCreateBeer reuses a global catalog row matched by case-insensitive
// name + brewery, or inserts a new beer. Style/ABV from the request are used
// only when inserting; existing master rows are left unchanged.
func findOrCreateBeer(ctx context.Context, tx pgx.Tx, userID string, beer *model.CreateBeerRequest) (string, error) {
	name := strings.TrimSpace(beer.Name)
	brewery := strings.TrimSpace(beer.Brewery)

	var id string
	err := tx.QueryRow(ctx,
		`SELECT id FROM beers
		 WHERE lower(name) = lower($1) AND lower(coalesce(brewery, '')) = lower($2)
		 LIMIT 1`,
		name, brewery,
	).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}

	err = tx.QueryRow(ctx,
		`INSERT INTO beers (name, brewery, style, abv, created_by)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, brewery, beer.Style, beer.ABV, userID,
	).Scan(&id)
	return id, err
}

func loadBeerByID(ctx context.Context, db *pgxpool.Pool, beerID string) (*model.Beer, error) {
	var b model.Beer
	err := db.QueryRow(ctx,
		`SELECT id, name, brewery, style, abv, created_by, created_at
		 FROM beers WHERE id = $1`, beerID,
	).Scan(&b.ID, &b.Name, &b.Brewery, &b.Style, &b.ABV, &b.CreatedBy, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func attachBeerAndPhotos(ctx context.Context, db *pgxpool.Pool, rev *model.Review) error {
	beer, err := loadBeerByID(ctx, db, rev.BeerID)
	if err != nil {
		return err
	}
	rev.Beer = beer
	reviews := []model.Review{*rev}
	if err := loadPhotos(ctx, db, reviews); err != nil {
		return err
	}
	*rev = reviews[0]
	return nil
}
