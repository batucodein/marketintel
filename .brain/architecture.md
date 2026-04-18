---
type: architecture
updated: 2026-04-11
---

# Architecture

## Overview

Full-stack monolith with clean internal boundaries. Go backend serves a REST API consumed by a Next.js frontend. Three-layer backend: HTTP handlers → service/pipeline → repository. AI calls routed through a provider abstraction that supports Azure OpenAI, Anthropic, and Google Gemini with Redis-backed response caching.

## Components

### Discovery Pipeline (`internal/discovery/`)
The core value — a four-stage pipeline that transforms a product description into scored leads:
1. **Stage 1 — HS Detection:** Product text → AI detects HS codes (trade classification)
2. **Stage 2 — Market Ranking:** HS code → Comtrade trade data + REST Countries demographics → TOPSIS multi-criteria ranking of countries
3. **Stage 3 — Importer Discovery:** Country → Tendata API or Excel import → city-level breakdown of importers + AI market summary
4. **Stage 4 — Lead Scoring:** Businesses → AI classification (relevant/not) → AI scoring (0-100 with rationale, strengths, weaknesses, approach)

### AI Platform (`internal/platform/ai/`)
Provider-agnostic AI routing. Each task type (classification, scoring, analysis, hs_detect) maps to a specific model and temperature. AI responses cached in Redis keyed by (provider, model, system prompt, user prompt, temperature). All calls logged to `ai_request_log` for cost tracking.

### Auth (`internal/auth/`)
JWT with access token (30 min) + refresh token (7 days). Bcrypt password hashing. Middleware validates tokens on protected routes.

### Frontend (`frontend/src/`)
Next.js App Router with route groups: `(auth)` for login/register, `(dashboard)` for protected pages. SWR for data fetching with automatic deduplication. AuthProvider context manages tokens and refresh logic.

## Data Flow

```
User enters product → POST /discover
  → AI detects HS codes → saves Search record

User selects HS code → POST /discover/{id}/markets
  → Comtrade API (trade volumes) + REST Countries (GDP, population)
  → TOPSIS ranking → returns scored countries

User selects country → POST /discover/{id}/explore
  → Tendata API (importers) OR Excel upload
  → AI generates market summary → city breakdown

User generates leads → POST /discover/{id}/leads (async)
  → AI classifies businesses (batches of 10)
  → Google Places enrichment (optional)
  → AI scores each lead → stores LeadScore records
```

## Infrastructure

- Docker Compose: Go API (:8000) + PostgreSQL 16 (:5433) + Redis 7 (:6379)
- DB and Redis run in Docker, Go backend runs locally via `go run` during development
- Migrations auto-run on startup
- Frontend runs separately via `next dev` (:3000)
