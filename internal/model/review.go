package model

import "time"

type Review struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	BeerID     string    `json:"beer_id"`
	Rating     float64   `json:"rating"`
	ReviewText string    `json:"review_text"`
	TastedAt   time.Time `json:"tasted_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Photos []ReviewPhoto `json:"photos"`
	Beer   *Beer         `json:"beer,omitempty"`
	User   *User         `json:"user,omitempty"`
}

type ReviewPhoto struct {
	ID         string `json:"id"`
	ReviewID   string `json:"review_id"`
	StorageKey string `json:"storage_key"`
	URL        string `json:"url,omitempty"`
	SortOrder  int    `json:"sort_order"`
}

type CreateReviewRequest struct {
	BeerID     string   `json:"beer_id"`
	Beer       *CreateBeerRequest `json:"beer,omitempty"`
	Rating     float64  `json:"rating"`
	ReviewText string   `json:"review_text"`
	TastedAt   *time.Time `json:"tasted_at"`
	PhotoKeys  []string `json:"photo_keys"`
}

type UpdateReviewRequest struct {
	Rating     *float64   `json:"rating"`
	ReviewText *string    `json:"review_text"`
	TastedAt   *time.Time `json:"tasted_at"`
	PhotoKeys  []string   `json:"photo_keys"`
	// Beer is accepted for older clients but ignored: catalog rows are
	// global master data and must not change via pour edits.
	Beer *CreateBeerRequest `json:"beer,omitempty"`
}

type PaginatedResponse struct {
	Data       interface{} `json:"data"`
	TotalCount int         `json:"total_count"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
}
