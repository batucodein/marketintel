---
type: patterns
updated: 2026-04-11
---

# Patterns

## Dependency Injection
All services wired in `cmd/server/main.go`. Each service/handler receives dependencies via constructor (`NewXxx(deps)`). No global state, no service locator.

## Error Handling
- Services return `(result, error)` — never panic.
- Wrap errors with context: `fmt.Errorf("functionName: %w", err)`.
- HTTP layer maps errors to status codes via `httputil` helpers.
- Structured logging with `slog` throughout.

## Concurrency
- External API calls run concurrently using `golang.org/x/sync/errgroup`.
- Context-based cancellation propagated through all layers.
- AI classification runs in batches of 10 with goroutines.

## Frontend Data Fetching
- SWR for all GET requests (deduplication, caching, revalidation).
- Typed API clients in `lib/api/` — one file per backend domain.
- AuthProvider handles token refresh on 401 automatically.

## AI Prompts
- Prompts defined as Go functions in `internal/platform/ai/prompts/`.
- Each prompt function returns `(systemPrompt, userPrompt)`.
- Prompts are versioned implicitly through code — no separate prompt store.

## Database Access
- pgx/v5 with connection pooling (`pgxpool.Pool`).
- Queries written as raw SQL (no ORM).
- Migrations in `migrations/` — numbered, with up/down pairs.
- Auto-migrate on startup.

## Testing
- Integration tests against real PostgreSQL (Docker via testcontainers preferred).
- No mocks for database layer.
