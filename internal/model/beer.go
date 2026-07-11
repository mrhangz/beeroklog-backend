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

	// Optional catalog pack shot (can / bottle). Storage key is the S3 object;
	// image_url is the API path clients load in <img> tags.
	ImageStorageKey string `json:"image_storage_key,omitempty"`
	ImageURL        string `json:"image_url,omitempty"`

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
	// ImageStorageKey: omit = unchanged; "" = clear; non-empty = set key from upload.
	ImageStorageKey *string `json:"image_storage_key"`
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
