package model

import "time"

type AdminUserPatch struct {
	IsActive     *bool
	IsAdmin      *bool
	PasswordHash *string
}

type User struct {
	ID              string    `json:"id"`
	Username        string    `json:"username"`
	Email           string    `json:"email"`
	PasswordHash    string    `json:"-"`
	DefaultCurrency string    `json:"defaultCurrency"`
	IsAdmin         bool      `json:"isAdmin"`
	IsActive        bool      `json:"isActive"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
