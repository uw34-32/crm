// Package model holds the types shared between the store and the API.
package model

import "time"

// User is safe to serialize: it never carries the password hash.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
