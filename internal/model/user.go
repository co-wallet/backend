package model

import "time"

type AdminUserPatch struct {
	IsActive     *bool
	IsAdmin      *bool
	PasswordHash *string
}

type User struct {
	ID              string
	Username        string
	Email           string
	PasswordHash    string
	DefaultCurrency string
	IsAdmin         bool
	IsActive        bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
