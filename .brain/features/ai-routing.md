---
type: feature
updated: 2026-04-19
---

# AI Provider Routing

## Overview

All AI calls go through a single `ai.Router` that maps a task name (e.g., `"scoring"`, `"classification"`, `"product_name"`, `"crossmatch"`) to a `{Provider, Model, Temperature, MaxTokens}` config. Providers are Azure OpenAI (primary), Anthropic Claude, and Google Gemini. Callers don't know or care which vendor serves the request — they just pass a task name to `router.CompleteJSON`.

Cache layer is Redis-backed; gracefully tolerates connection-refused (Cloud Run doesn't have Redis deployed). Cost per request is logged with `user_id` and `search_id` to `ai_request_log` for the dashboard.

## Registered Tasks

After the 2026-04-18 redesign:

| Task | Model | Purpose |
|------|-------|---------|
| `classification` | gpt-5.4-mini | Business type/industry labeling with `data_completeness` |
| `scoring` | gpt-5.4-mini | Lead scoring on 5 dimensions with server-side caps |
| `product_name` | gpt-5.4-nano | Grounded market product-name summarization + origin singleness check |
| `crossmatch` | gpt-5.4-nano | Tendata-to-Google-Places name/address matching; empty match allowed |

Removed in redesign: `hs_detect`, `market_summary`, `analysis`, `queries`. Each removal was backed by a decision:
- `hs_detect` → Excel already has HS codes; don't re-derive from free text. See [[decisions.md#collapse-4-stage-discovery-into-excel-only-upload]].
- `market_summary` + `analysis` → removed with the market-analysis feature. See [[decisions.md#delete-market-analysis-feature-analyze-go]].
- `queries` → Google Places discovery mode isn't used anymore.

## Key Files

- `backend-go/internal/platform/ai/router.go` — `Router`, `DefaultTasks`, `SetLogFunc`
- `backend-go/internal/platform/ai/cache.go` — Redis cache (tolerant)
- `backend-go/internal/platform/ai/context.go` — `WithUserID`, `WithSearchID` for request attribution
- `backend-go/internal/platform/ai/azure.go` — Azure provider
- `backend-go/internal/platform/ai/claude.go` — Claude provider
- `backend-go/internal/platform/ai/gemini.go` — Gemini provider
- `backend-go/internal/platform/ai/prompts/base.go` — shared `NoHallucinationPreamble`
- `backend-go/internal/platform/ailog/repository.go` — persistence + cost summary query

## Timeline

- **2026-04-18** — Cleaned up task registry (removed 4 tasks, added `product_name`).
- **2026-04-07** — Added search-ID tagging on AI logs (migration 4). See [[history.md#ai-cost-tracking-per-discovery]].
- **2026-04-04** — Router + multi-provider routing established in the Go rewrite.

## Current State

Working. Primary provider is Azure OpenAI; Claude and Gemini are wired up but fall back targets. Cache works locally, silently disabled in production.
