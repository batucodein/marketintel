---
type: patterns
updated: 2026-04-19
---

# Patterns

## Pluggable messaging transports via a Channel interface

External services we send messages through (Gmail now, WhatsApp/Outlook/SMTP later) live behind `outreach/channel/interface.go::Channel`. Conversation, campaign, and sequence code calls only `Channel.Send` and `Channel.ListNewMessages`; they never import a concrete channel package.

Adding a new channel:
1. Create `outreach/channel/<name>/<name>.go` implementing the interface.
2. Register a factory on `DefaultRegistry` in `main.go` during boot: `channel.DefaultRegistry.Register(domain.ChannelTypeFoo, foo.NewFactory(...))`.
3. Store user-specific config in `user_channels.config_encrypted` (JSONB) — factory reads and decrypts on construction.

Do not short-cut this by passing a `*gmail.Service` around. The whole point is that the rest of the app doesn't know or care which transport.

## Public OAuth callback sits above the authenticated Mount

Google (and any other OAuth provider) calls our redirect URL without our JWT. Those callback routes must live at a specific path registered BEFORE the authenticated `Mount("/outreach", ...)`. Chi's router matches specific routes before sub-trees, so a public `Get("/outreach/channels/gmail/callback", ...)` wins over the protected `Mount`.

User identification inside a public callback uses the OAuth `state` parameter — store the state→userID mapping server-side (in-memory TTL store is fine for single-instance; move to Redis once multi-instance matters).

Never, ever trust `state` without verifying it came from a state we issued. `channel/handler.go:stateStore` does this.

## Secrets at rest: AES-256-GCM via platform/crypto

OAuth tokens, SMTP credentials, and anything comparable go through `crypto.Cipher.Encrypt/Decrypt` before hitting the DB. Key comes from `TOKEN_ENCRYPTION_KEY` (32-byte base64) in production, derived from `SECRET_KEY` via SHA-256 in dev. Don't use `pgcrypto` or app-level base64 as a substitute.

If the key rotates, old ciphertext becomes unreadable — users must re-authenticate. Acceptable for our scale.

## Anti-hallucination: NoHallucinationPreamble on every AI prompt

Every AI system prompt is wrapped by `prompts.withPreamble()` which prepends `NoHallucinationPreamble` — a short block instructing the model to use only provided data, output "unknown" for missing fields, and include a `data_completeness` score (0.0–1.0) in the response.

See `internal/platform/ai/prompts/base.go`. Don't add a new prompt without this.

## Anti-hallucination: server-side data-completeness caps

Even if the AI returns `overall_score: 95` for a thin-data lead, the server caps it:

- `data_completeness ≤ 0.2` → `overall_score ≤ 45`
- `data_completeness ≤ 0.4` → `overall_score ≤ 60`
- `data_completeness ≤ 0.6` → `overall_score ≤ 75`

Enforced in `internal/discovery/scoring.go`. The prompt also instructs the model to self-cap, but the server is the authority.

## Deterministic helpers vs AI calls — clear separation

Anything that can be computed from the data deterministically (frequency counts, percentiles, ISO country mapping) lives in a pure-Go helper with unit tests. AI is reserved for summarization / classification / scoring where judgment is needed.

Example: `market_metadata.go` is entirely pure Go (tested in `market_metadata_test.go`). Product-name summarization goes through `product_name.go` AI prompt. Don't blur this line — if a new derivation can be done without AI, do it without AI.

## Per-market async enrichment via goroutines

`ProcessExcel` returns synchronously once markets are created and businesses stored. Each market then runs `EnrichAndScore` in its own goroutine (via `runEnrichAndScore` wrapper). Completion is tracked by in-memory map keyed by `searchID`, and when all markets for a search finish, search status moves to `completed`.

This keeps the upload response fast (~1-3s) while long work (3-10 min) happens in the background. Frontend polls every 3s to watch status.

Constraint: this only works on Cloud Run because we enabled `--no-cpu-throttling`. Without it, CPU is taken away when the HTTP handler returns and goroutines die.

## Concurrency limits on external calls

- Google Places verification: max 5 concurrent + 100ms delay between calls (respects Places API quotas)
- Website scraping: max 10 concurrent
- AI scoring: 8 businesses per batch, max 3 concurrent batches

Pattern: `sem := make(chan struct{}, N)` + `errgroup.WithContext`. See `pipeline.go` `verifyWithGooglePlaces` and `scrapeEmails`.

## Cache errors are WARN, never fatal

Redis is optional. On Cloud Run we don't run Redis and `cache get error: connection refused` logs fire continuously. The cache layer returns a miss on error and the caller proceeds. Don't wire Redis errors into request failures.

## Flexible Excel column lookup

The parser tries multiple column name variants for every field. Example:
```go
ConsigneeCountry: ci.get(row, "Consignee Country(EN)", "Consignee Country", "Destination Country", "Country"),
```

Tendata and other customs vendors use inconsistent headers. Always add variants when a real-world file breaks the parser rather than hardcoding one name.

## ISO-2 fallback code "ZZ" for unknown

When a ConsigneeCountry can't be mapped to ISO-2, we bucket it as `"ZZ"` (officially-unassigned ISO code). `ProcessExcel` skips the `"ZZ"` group and emits a warning rather than crashing. Never use a >2-char fallback like `"UNKNOWN"` — the `country_code` column is `varchar(3)` and 7 chars will produce `SQLSTATE 22001`.

## Auth via JWT + refresh rotation

Access token (short-lived) in-memory on frontend; refresh token in localStorage. On 401, the client tries `POST /auth/refresh` once, dedupes concurrent refresh attempts via a shared promise, and retries the original request. See `frontend/src/lib/api/client.ts`.

## Structured JSON logging (slog)

Backend logs via `slog` with a JSON handler. Every log has a `msg` field, optional `error`, and a log-level. This is what Cloud Logging ingests. Don't add `fmt.Println` — use `slog.Info/Warn/Error` so logs stay queryable.

## Migrations apply on boot, not in a separate CI step

`db.RunMigrations` runs every time the backend starts (local or cloud). The embedded migration FS (`migrations/embed.go`) ships with the binary. If a migration fails, the server exits with a clear error. Don't create out-of-band migration scripts.

## TaskCreate for multi-step work, not ad-hoc tracking

For any implementation that spans 3+ steps, use the TaskCreate tool so progress is visible in the UI. Mark tasks `in_progress` when starting and `completed` immediately when done — not in batches at the end.

## Pre-push safety: verify secrets and nested repos before `git add -A`

Before the initial push to GitHub we discovered an embedded `.git` inside `frontend/` and had to clean it out. Always scan for nested git repos and `.env` content before a broad `git add`. Gitignore covers `.env`, `DEPLOYMENT.md`, `.claude/`.
