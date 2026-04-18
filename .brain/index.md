---
type: index
updated: 2026-04-11
---

# MarketIntel

AI-powered B2B market intelligence platform for exporters. Discovers, analyzes, and scores potential international buyers through a four-stage pipeline combining trade data, external APIs, and LLM-driven scoring.

## Tech Stack

- **Backend:** Go 1.26 with Chi v5 router
- **Frontend:** Next.js 16 (React 19) with shadcn/ui, Tailwind CSS v4, SWR
- **Database:** PostgreSQL 16 via pgx/v5
- **Cache:** Redis 7 via go-redis/v9
- **AI:** Multi-provider routing — Azure OpenAI (primary: gpt-5.4-mini/nano), Anthropic Claude, Google Gemini
- **Auth:** JWT (access + refresh tokens) via golang-jwt/v5
- **Migrations:** golang-migrate/v4, auto-run on startup
- **Containerization:** Docker Compose (API + PostgreSQL + Redis)

## Key Directories

- `backend-go/cmd/server/` — Entry point, dependency injection
- `backend-go/internal/discovery/` — Four-stage pipeline (core business logic)
- `backend-go/internal/platform/ai/` — Multi-provider AI routing + caching + prompts
- `backend-go/internal/datasource/` — External API connectors (Comtrade, Google Places, Tendata)
- `backend-go/internal/auth/` — JWT auth, user repository
- `backend-go/internal/domain/` — Core entities and interfaces
- `backend-go/migrations/` — SQL up/down migrations
- `frontend/src/app/` — Next.js App Router pages
- `frontend/src/components/` — React components (ui/, discover/, leads/, markets/)
- `frontend/src/lib/api/` — HTTP clients for each backend endpoint

## Team

- Batuhan — sole developer, owns all decisions
