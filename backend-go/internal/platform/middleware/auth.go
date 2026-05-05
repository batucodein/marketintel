package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// JWTValidator validates a JWT token and returns the user ID.
type JWTValidator interface {
	ValidateToken(tokenStr string, expectedType string) (uuid.UUID, error)
}

// UserFetcher fetches a user by ID and injects into context.
type UserFetcher interface {
	FetchAndInject(ctx context.Context, userID uuid.UUID) (context.Context, error)
}

func RequireAuth(jwt JWTValidator, fetcher UserFetcher) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				http.Error(w, `{"detail":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			userID, err := jwt.ValidateToken(token, "access")
			if err != nil {
				http.Error(w, `{"detail":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx, err := fetcher.FetchAndInject(r.Context(), userID)
			if err != nil {
				http.Error(w, `{"detail":"user not found"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractToken pulls the access token from either the Authorization header
// (preferred) or the ?access_token= query parameter. The query-param path
// exists because EventSource — used for /outreach/events — cannot set
// custom headers. Anywhere else, prefer the header.
func extractToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	if t := r.URL.Query().Get("access_token"); t != "" {
		return t
	}
	return ""
}
