package domain

import (
	"time"

	"github.com/google/uuid"
)

type ProductCategory struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	Name          string     `json:"name" db:"name"`
	ParentID      *uuid.UUID `json:"parent_id" db:"parent_id"`
	Slug          string     `json:"slug" db:"slug"`
	Keywords      []string   `json:"keywords" db:"keywords"`
	HSCode        *string    `json:"hs_code" db:"hs_code"`
	HSDescription *string    `json:"hs_description" db:"hs_description"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}
