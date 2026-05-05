// Package internalsched hosts the /internal/scheduler/tick endpoint that
// Cloud Scheduler calls every minute. The handler fans out to:
//   - inbound polling (replaces the standalone poller goroutine)
//   - campaign drafter (P2)
//   - campaign send scheduler (P2)
//   - sequence engine (P3)
//
// All work runs inline in the request — Cloud Run holds the connection until
// the tick returns. For typical loads (≤1000 channels, ≤500 due rows) this is
// well under the 30-minute Cloud Run timeout.
package internalsched

import (
	"crypto/subtle"
	"net/http"

	"github.com/batuhan/marketintel/internal/platform/httputil"
)

// AuthMiddleware enforces X-Internal-Token on the wrapped routes.
// Rejects with 401 if the header is absent or doesn't match the configured
// token. Constant-time comparison to avoid timing attacks.
func AuthMiddleware(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				httputil.WriteError(w, http.StatusServiceUnavailable, "internal endpoint disabled: INTERNAL_API_TOKEN not set")
				return
			}
			got := r.Header.Get("X-Internal-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				httputil.WriteError(w, http.StatusUnauthorized, "invalid internal token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
