package domain

import "time"

type MediaItem struct {
	Type string `json:"type" bson:"type"`
	URL  string `json:"url"  bson:"url"`
}

type Tweet struct {
	ID        string      `json:"id"`
	UserID    string      `json:"user_id"`
	Text      string      `json:"text"`
	Media     []MediaItem `json:"media,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}
