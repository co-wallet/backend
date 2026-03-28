package model

import "time"

type Invite struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Token     string     `json:"token"`
	CreatedBy string     `json:"createdBy"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
	ExpiresAt time.Time  `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
}
