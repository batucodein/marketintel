---
type: feature
updated: 2026-04-19
---

# Dashboard + Cost Tracking

## Overview

Serves the dashboard overview (total stats, top leads) and the AI cost breakdown (per-search, per-model, daily). Every AI call is logged to `ai_request_log` with `user_id`, `search_id`, `provider`, `model`, tokens, cost. The cost page joins against `searches` so the user sees cost per upload (via `source_file_name`), status, and lead count.

## Endpoints

- `GET /dashboard/overview` — high-level stats (markets, leads, AI spend)
- `GET /dashboard/top-leads` — across all markets, ordered by overall_score
- `GET /dashboard/ai-costs` — summary with by_search, by_day, by_model breakdowns

## Key Files

- `backend-go/internal/dashboard/handler.go` — endpoint wiring
- `backend-go/internal/platform/ailog/repository.go` — cost queries
- `backend-go/internal/domain/search.go` — `SearchCostSummary`, `DailyCostSummary`, `ModelCostSummary`
- `frontend/src/app/(dashboard)/page.tsx` — overview page
- `frontend/src/app/(dashboard)/costs/page.tsx` — cost breakdown page

## Timeline

- **2026-04-18** — `SearchCostSummary.product_query` renamed to `source_file_name` as part of the Excel-only redesign.
- **2026-04-07** — search_id FK on AI logs (migration 4). See [[history.md#ai-cost-tracking-per-discovery]].

## Current State

Working. Dashboard is read-only. Cost summary shows the last 30 days of aggregate data.
