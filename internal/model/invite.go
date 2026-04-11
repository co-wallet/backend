package model

import "time"

type Invite struct {
	ID        string
	Email     string
	Token     string
	CreatedBy string
	UsedAt    *time.Time
	ExpiresAt time.Time
	CreatedAt time.Time
}
