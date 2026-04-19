---
type: index
updated: 2026-04-19
---

# MarketIntel

B2B trade intelligence platform for exporters. Users upload customs-shipment Excel files; the system splits records by destination country, creates one market per destination, enriches importers with Google Places + website scraping, verifies emails via SMTP, classifies + scores leads with multi-provider AI, and presents the results as scored lead lists. Anti-hallucination is enforced server-side with data-completeness caps on AI scores.

Currently deployed to Cloud Run (GCP) with a Supabase PostgreSQL backend. The flow is intentionally Excel-only — no product-name input, no HS-code detection, no trade-data ranking — which was the outcome of the redesign on 2026-04-18.

## Tech Stack

- **Backend:** Go 1.26 with Chi v5 router, pgx/v5 for Postgres, golang-migrate for migrations, JWT auth, httprate for rate limiting
- **Frontend:** Next.js 16 (App Router), React 19, shadcn/ui, SWR, TailwindCSS v4
- **Database:** PostgreSQL 17 on Supabase (cloud) / Postgres 16 via Docker (local)
- **Cache:** Redis 7 (local only — Cloud Run skips cache, tolerates connection refused)
- **AI:** Multi-provider router — Azure OpenAI (primary: gpt-5.4-mini / nano), Anthropic Claude, Google Gemini
- **Auth:** JWT access + refresh tokens
- **External:** Google Places API (business verification), SMTP (email deliverability check), website scraping (custom)
- **Deploy:** Cloud Run (backend + frontend as containers), Cloud Build for images, Artifact Registry
- **Repo structure:** monorepo (`backend-go/`, `frontend/`)

## Key Directories

- `backend-go/cmd/server/` — entry point, dependency injection
- `backend-go/internal/discovery/` — Excel-upload-only pipeline (the core feature)
- `backend-go/internal/platform/ai/` — multi-provider AI routing + prompts
- `backend-go/internal/datasource/` — Google Places, website scraper, SMTP email verify (Comtrade/Tendata/RestCountries files still present but unused)
- `backend-go/internal/auth/` — user repo, JWT manager
- `backend-go/internal/scoring/` — market + leads serving, MCDM scoring
- `backend-go/internal/dashboard/` — AI cost overview endpoints
- `backend-go/migrations/` — 7 migrations, auto-applied on boot
- `frontend/src/app/(dashboard)/discover/` — upload page + progress page
- `frontend/src/app/(dashboard)/markets/` — markets list + detail + leads
- `frontend/src/components/markets/market-card.tsx` — the PM-approved card layout

## Team

- Batuhan (solo dev, owns all decisions)

## Live URLs

- Frontend: https://marketintel-frontend-631545360913.us-central1.run.app
- Backend: https://marketintel-631545360913.us-central1.run.app
- GCP project: `marketintel-493113` (us-central1)
- Supabase project: `marketIntel` (US East)

Full deployment details are in `DEPLOYMENT.md` (gitignored).
