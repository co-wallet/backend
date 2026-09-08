package model

import "time"

type Tag struct {
	ID        string
	UserID    string
	Name      string
	Hidden    bool
	CreatedAt time.Time
}

type TagWithCount struct {
	Tag
	TxCount int
}
