---
type: architecture
updated: 2026-04-19
---

# Architecture

## Overview

Request-driven Go monolith on Cloud Run, scale-to-zero, with CPU always allocated so async enrichment goroutines can run after the HTTP handler returns. Frontend polling every 3 seconds is what keeps the instance warm while background work completes.

One upload = one `search` row + N `market` rows (one per destination country in the Excel). Each market is enriched independently in a goroutine so large multi-destination uploads run in parallel.

## Request Flow — Excel Upload

```
POST /discover/  (multipart: file)
   │
   ▼
auth middleware  (JWT check, rate-limit 100/min global)
   │
   ▼
discovery.Handler.UploadExcel
   │
   ├── repo.CreateSearch — rows at status="processing"
   │
   ├── pipeline.ProcessExcel  (sync, ~1-3s)
   │     ├── ParseTendataExcelRaw     — xlsx reader → []ShipmentRecord
   │     ├── SplitByDestination       — group by ConsigneeCountry → []ShipmentGroup
   │     │     (rows with unrecognized country bucket to "ZZ" and are skipped with a warning)
   │     ├── For each group:
   │     │     ├── DeriveMarketMetadata — pure Go: top HS code, origin share,
   │     │     │                           5-95 percentile date range, counts
   │     │     ├── repo.CreateMarket   — INSERT with full metadata
   │     │     ├── AggregateRecords    — per-consignee rollup
   │     │     ├── repo.StoreBulkBusinesses
   │     │     └── go runEnrichAndScore (goroutine)
   │     └── repo.UpdateSearchQuery    — persist {market_ids, source_file_name}
   │
   └── Return 202 { search_id, markets, status: "enriching" }

Async per market (goroutine, background ctx, 30-min timeout):
   │
   ├── deriveProductName AI  (grounded — reads top 20 product descriptions)
   ├── repo.UpdateMarketProductName
   ├── verifyWithGooglePlaces  (5 concurrent, 100ms delay)
   ├── scrapeEmails  (10 concurrent: emails, phones, whatsapp, socials)
   ├── SMTP email verification  (best-effort per business)
   ├── classifier.ClassifyBatch  (AI, uses derived HS + product name)
   └── scorer.ScoreBatch  (AI, marketCtx=nil, data_completeness caps enforced server-side)
   │
   ▼
trackCompletion — when all goroutines for a search done → status="completed"
```

Frontend polls `GET /discover/{id}` every 3s. On `status=completed` it redirects to `/markets`.

## Components

### Discovery Module (`internal/discovery/`)

The core of the app. Strict separation between the deterministic (pure-Go) helpers and the AI-driven enrichment:

- **`market_metadata.go`** — `SplitByDestination`, `DeriveMarketMetadata`, `countryToISO2`, `parseArrivalDate`, percentile computation. No AI, no hallucination.
- **`excel_import.go`** — xlsx parsing, per-consignee aggregation, trust-tier tagging. Column name lookup is flexible (tries several variants).
- **`excel_pipeline.go`** — `ProcessExcel` (sync) + `runEnrichAndScore`/`EnrichAndScore` (async) + `trackCompletion` for multi-market status roll-up.
- **`excel_handler.go`** — `POST /discover/` and `GET /discover/{id}/markets`.
- **`handler.go`** — `Routes()` + `GetSearch`. Minimal.
- **`pipeline.go`** — shared enrichment: `verifyWithGooglePlaces`, `scrapeEmails`, `PlacesEnrichment` type.
- **`classification.go`** — AI classifier (kept from pre-redesign, works with derived HS/product).
- **`scoring.go`** — AI scorer with server-side data_completeness caps (≤0.2 → max 45, ≤0.4 → max 60, ≤0.6 → max 75).
- **`repository.go`** — Postgres queries; `CreateMarket` always INSERTs (no upsert).

### Platform / AI (`internal/platform/ai/`)

- **`router.go`** — task → provider+model mapping. Currently registered: `classification`, `scoring`, `product_name`, `crossmatch`. `hs_detect`, `market_summary`, `analysis`, `queries` were all removed in the 2026-04-18 redesign.
- **`cache.go`** — Redis caching; gracefully tolerates connection refused (Cloud Run doesn't have Redis). Logs WARN, returns cache miss.
- **`context.go`** — `WithUserID`, `WithSearchID` to thread auth metadata through the AI call for cost logging.
- **`prompts/base.go`** — `NoHallucinationPreamble` prepended to every system prompt.
- **`prompts/score.go`** — the paranoid lead-scoring prompt with explicit dimension bands and data_completeness gate.
- **`prompts/classify.go`** — business classification with mandatory `data_completeness` field.
- **`prompts/crossmatch.go`** — Tendata-to-Google-Places matching; empty result is acceptable (prevents false matches).
- **`prompts/product_name.go`** — grounded product name summarization + origin-singleness sanity check. Added in the redesign.

### Scoring Module (`internal/scoring/`)

Serves `GET /markets`, `GET /markets/{id}`, `GET /markets/{id}/leads`, `DELETE /markets/{id}`, plus the MCDM (`rank-markets` / `rankings`) endpoints. The old `POST /markets/{id}/analysis` endpoint was deleted in the redesign.

### Auth + Dashboard

- JWT access + refresh, bcrypt passwords. Rate-limited: 10/min on `/auth/*`.
- Dashboard serves `/dashboard/overview`, `/dashboard/top-leads`, and the AI cost summary joined from `ai_request_log` + `searches`.

## Data Model (migrations 1-7)

- `users` — email, hashed_password, home_country, tier, api_calls_remaining
- `product_categories` — static reference data
- `businesses` — core entity, enriched over time. Notable columns: `shipment_data` (JSONB, migration 5), `website_data` (JSONB, migration 6), `email_verified` (bool, migration 6), `google_place_id` (unique idx, migration 2), `google_types`/`opening_hours`/`rating` (migration 3)
- `markets` — extended in migration 7 with `derived_product_name`, `dominant_hs_code`, `all_hs_codes TEXT[]`, `origin_country`, `origin_countries TEXT[]`, `origin_share NUMERIC(5,4)`, `shipment_from_date`, `shipment_to_date`, `importer_count`, `shipment_count`, `source_file_name`, `uploaded_at`
- `business_markets` — M:N with `relevance_score`
- `lead_scores` — unique (business_id, market_id, user_id); holds overall_score + 5 dimensions + rationale/strengths/weaknesses
- `searches` — search_type, query (JSONB holds `ExcelSearchState{market_ids, source_file_name, uploaded_at, warnings}`), status
- `ai_request_log` — cost tracking with FK to searches

Migration 7 also runs `UPDATE searches SET status='failed'` for legacy statuses (`pending`, `hs_detected`, `markets_ranked`, `explored`) and `DELETE FROM market_analyses` since that feature was removed.

## Infrastructure

- **Local dev:** `docker compose up` brings up Postgres 16 + Redis 7. Go backend runs locally via `go run ./cmd/server`. Next.js via `npm run dev`. Migrations auto-apply on boot.
- **Cloud:** Cloud Run (backend + frontend) in `us-central1`. Images in Artifact Registry. `--no-cpu-throttling` + `--timeout=1800` + `--min-instances=0` — CPU stays allocated while instance is alive (so async goroutines can finish), instance scales to zero when idle. Request timeout 30 minutes.
- **Database:** Supabase Postgres 17 (cloud). Local Supabase not used; local uses docker postgres.
- **Billing:** GCP free trial (TRY 13K credit, expires June 2026).
- **Monitoring:** Cloud Logging (structured JSON from slog). No external APM.

## Out of Scope (deliberately)

- UN Comtrade trade-flow data (removed in redesign — source stayed in code but unwired)
- Tendata API live fetch (same)
- REST Countries enrichment (same)
- AI market analysis (removed — was hallucination-prone without trade data)
- Email marketing / outreach (product spec mentions it but not implemented yet)
