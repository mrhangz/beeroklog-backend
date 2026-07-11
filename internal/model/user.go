package model

import "time"

type User struct {
	ID             string    `json:"id"`
	Email          string    `json:"email"`
	DisplayName    string    `json:"display_name"`
	AvatarURL      string    `json:"avatar_url"`
	IsAdmin        bool      `json:"is_admin"`
	AuthProvider   string    `json:"-"`
	AuthProviderID string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}
