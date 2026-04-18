package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID                       uuid.UUID `json:"id" db:"id"`
	Email                    string    `json:"email" db:"email"`
	HashedPassword           string    `json:"-" db:"hashed_password"`
	CompanyName              *string   `json:"company_name" db:"company_name"`
	HomeCountry              *string   `json:"home_country" db:"home_country"`
	DefaultProductCategories []string  `json:"default_product_categories" db:"default_product_categories"`
	SubscriptionTier         string    `json:"subscription_tier" db:"subscription_tier"`
	APICallsRemaining        int       `json:"api_calls_remaining" db:"api_calls_remaining"`
	CreatedAt                time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time `json:"updated_at" db:"updated_at"`
}
