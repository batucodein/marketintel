---
type: feature
updated: 2026-04-19
---

# Discovery (Excel upload)

## Overview

The core feature. A user uploads a customs-shipment Excel; the backend parses it, splits records by destination country, creates one `market` row per destination with deterministically-derived metadata (dominant HS code, origin country, date range, counts), stores the per-consignee importers as `business` rows, and spawns one async goroutine per market to enrich + classify + score.

The user sees a progress page that polls `GET /discover/{id}` every 3 seconds while the `search.status` moves through `processing → enriching → scoring → completed`. When all market goroutines finish, the frontend redirects to `/markets`.

## Endpoints

- `POST /discover/` — multipart upload, returns 202 with the list of created markets.
- `GET /discover/{id}` — status + timestamps. Used by the progress page's poll.
- `GET /discover/{id}/markets` — the list of market IDs created by this upload (useful for the markets summary view).
- `GET /markets/{id}/leads` — lives in the scoring module, but it's the downstream page the user ends up on.

## Key Files

- `backend-go/internal/discovery/excel_handler.go` — `UploadExcel`, `GetMarkets`
- `backend-go/internal/discovery/excel_pipeline.go` — `ProcessExcel` (sync), `EnrichAndScore` (async), `trackCompletion`
- `backend-go/internal/discovery/excel_import.go` — xlsx parsing, per-consignee aggregation
- `backend-go/internal/discovery/market_metadata.go` — pure-Go derivation: `SplitByDestination`, `DeriveMarketMetadata`, `countryToISO2`, percentile dates
- `backend-go/internal/discovery/pipeline.go` — shared enrichment: Google Places verification, website scraping
- `backend-go/internal/discovery/scoring.go` — AI scorer with server-side data_completeness caps
- `backend-go/internal/discovery/classification.go` — AI classifier
- `backend-go/internal/discovery/repository.go` — Postgres operations
- `backend-go/internal/platform/ai/prompts/product_name.go` — grounded product-name summarization
- `backend-go/internal/platform/ai/prompts/score.go` — the paranoid scoring prompt
- `backend-go/migrations/000007_market_derived_metadata.up.sql` — schema extension for derived metadata
- `frontend/src/app/(dashboard)/discover/page.tsx` — upload form
- `frontend/src/app/(dashboard)/discover/[searchId]/page.tsx` — progress page
- `frontend/src/components/markets/market-card.tsx` — PM-approved card layout

## Timeline

- **2026-04-18** — Excel-only redesign shipped. 4-stage wizard replaced with single upload. See [[decisions.md#collapse-4-stage-discovery-into-excel-only-upload]] and [[history.md#excel-only-discovery-redesign-shipped]].
- **2026-04-18** — Multi-destination splitting (one upload → N markets). See [[decisions.md#split-multi-country-excels-into-multiple-markets]].
- **2026-04-18** — AI-generated product names with origin sanity check. See [[decisions.md#use-ai-for-grounded-summarization-not-invention]].
- **2026-04-18** — Fixed UNKNOWN country overflow bug. See [[bugs.md#unknown-country-fallback-overflowed-varchar3]].
- **2026-04-18** — Fixed Cloud Run CPU throttling killing async enrichment. See [[bugs.md#async-enrichment-silently-killed-by-cloud-run-cpu-throttling]].
- **2026-04-10** — Website enrichment upgraded (multiple emails, phones, social, contact form detection, SMTP verify). See [[history.md#website-enrichment-upgraded]].
- **2026-04-09** — Shipment data moved to dedicated JSONB column. See [[history.md#shipment-data-column-refactor]].

## Current State

Working in production (`https://marketintel-frontend-631545360913.us-central1.run.app`). End-to-end flow completes for typical uploads (< 500 businesses across < 10 destination countries) in 3-10 minutes.

Known limitations:
- If the user closes the browser tab mid-enrichment and the instance scales to zero before the goroutines complete, work is lost. Acceptable at current scale; tracked as a v2 concern.
- Duplicate uploads (same product + destination) create separate markets. See [[decisions.md#keep-separate-markets-on-duplicate-uploads-instead-of-merging]] — intentional, tracked as GitHub issue #2.
- Multi-origin Excels (importer-perspective lists) are assumed to have a dominant origin; sanity check in `product_name.go` flags when origin share < 85%. Proper multi-origin support is GitHub issue #1.
