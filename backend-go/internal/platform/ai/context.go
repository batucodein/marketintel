package ai

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const (
	ctxKeyUserID   ctxKey = iota
	ctxKeySearchID
)

// WithUserID returns a context with the user ID set for AI cost tracking.
func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, &userID)
}

// WithSearchID returns a context with the search ID set for AI cost tracking.
func WithSearchID(ctx context.Context, searchID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKeySearchID, &searchID)
}

// UserIDFromContext extracts user ID from context.
func UserIDFromContext(ctx context.Context) *uuid.UUID {
	v, _ := ctx.Value(ctxKeyUserID).(*uuid.UUID)
	return v
}

// SearchIDFromContext extracts search ID from context.
func SearchIDFromContext(ctx context.Context) *uuid.UUID {
	v, _ := ctx.Value(ctxKeySearchID).(*uuid.UUID)
	return v
}
