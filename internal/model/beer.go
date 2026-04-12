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
