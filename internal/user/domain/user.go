package domain

import "time"

type User struct {
	ID            string    `json:"id"`
	Username      string    `json:"username"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	OAuthProvider string    `json:"-"`
	OAuthSub      string    `json:"-"`
}
