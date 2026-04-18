package auth

import (
	"context"

	"github.com/google/uuid"
)

// UserFetcherAdapter adapts the auth module's UserRepository to satisfy
// the platform middleware's UserFetcher interface.
type UserFetcherAdapter struct {
	users UserRepository
}

func NewUserFetcherAdapter(users UserRepository) *UserFetcherAdapter {
	return &UserFetcherAdapter{users: users}
}

func (a *UserFetcherAdapter) FetchAndInject(ctx context.Context, userID uuid.UUID) (context.Context, error) {
	user, err := a.users.GetByID(ctx, userID)
	if err != nil {
		return ctx, err
	}
	return WithUser(ctx, user), nil
}
