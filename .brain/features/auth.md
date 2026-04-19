---
type: feature
updated: 2026-04-19
---

# Authentication

## Overview

JWT-based auth with refresh-token rotation. Email + password login. Rate-limited separately from the rest of the API (10 req/min/IP on `/auth/*` vs 100 global). Access tokens kept in frontend memory; refresh tokens in localStorage.

## Endpoints

- `POST /auth/register` — email + password + optional company_name + home_country
- `POST /auth/login` — returns access_token + refresh_token
- `POST /auth/refresh` — returns new pair; rotates the refresh token
- `GET /auth/me` — current user (requires Authorization header)
- `PATCH /auth/me` — update profile

## Key Files

- `backend-go/internal/auth/handler.go` — HTTP handlers
- `backend-go/internal/auth/service.go` — register/login/refresh logic
- `backend-go/internal/auth/jwt.go` — JWT signing + verification
- `backend-go/internal/auth/repository.go` — users table access
- `backend-go/internal/auth/user_fetcher.go` — adapter between middleware and repo
- `backend-go/internal/platform/middleware/auth.go` — `RequireAuth` middleware
- `frontend/src/lib/api/client.ts` — 401 → refresh → retry pattern
- `frontend/src/lib/providers/auth-provider.tsx` — React context for token state

## Timeline

- **2026-04-13** — Rate limiting added (global 100/min, auth 10/min). See [[decisions.md#rate-limit-auth-endpoints-more-strictly-than-rest-of-api]].
- **2026-04-04** — JWT auth established in Go rewrite. Before that the Python version had session-based auth.

## Current State

Working. Trivial to reset passwords via SQL if needed. No 2FA yet, no social login. Password reset flow via email is not implemented.
