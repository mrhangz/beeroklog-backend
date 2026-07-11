package model

import "time"

type Beer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Brewery   string    `json:"brewery"`
	Style     string    `json:"style"`
	ABV       *float64  `json:"abv"`
	CreatedBy *string   `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	AvgRating   *float64 `json:"avg_rating,omitempty"`
	ReviewCount *int     `json:"review_count,omitempty"`
}

type CreateBeerRequest struct {
	Name    string   `json:"name"`
	Brewery string   `json:"brewery"`
	Style   string   `json:"style"`
	ABV     *float64 `json:"abv"`
}

type UpdateBeerRequest struct {
	Name    *string  `json:"name"`
	Brewery *string  `json:"brewery"`
	Style   *string  `json:"style"`
	ABV     *float64 `json:"abv"`
}

// MergeBeersRequest keeps one catalog row and reassigns reviews from the others.
type MergeBeersRequest struct {
	KeepID   string   `json:"keep_id"`
	MergeIDs []string `json:"merge_ids"`
}

type BeerDuplicateGroup struct {
	Name    string `json:"name"`
	Brewery string `json:"brewery"`
	Beers   []Beer `json:"beers"`
}

type DedupeExactResult struct {
	GroupsMerged int `json:"groups_merged"`
	BeersRemoved int `json:"beers_removed"`
	ReviewsMoved int `json:"reviews_moved"`
}
