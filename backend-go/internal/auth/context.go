package auth

import (
	"context"

	"github.com/batuhan/marketintel/internal/domain"
)

type contextKey string

const userContextKey contextKey = "user"

func WithUser(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func UserFromContext(ctx context.Context) *domain.User {
	user, _ := ctx.Value(userContextKey).(*domain.User)
	return user
}
