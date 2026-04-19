---
type: history
updated: 2026-04-19
---

# History

## Outreach P1 backend complete — end-to-end Gmail flow works

**Date:** 2026-04-19
Conversation service, AI draft/reply prompts, and Gmail inbox polling worker landed. Backend can now drive a full cycle: connect Gmail → start conversation with AI-drafted opener → send → poller picks up replies within 2 min → user requests AI-suggested reply → send reply (threaded). Pipeline stage transitions happen automatically (`lead` → `contacted` on first send, `contacted` → `replied` on incoming reply). Two new AI prompts: `outreach_draft` and `outreach_reply` with outreach-specific guardrails (no invented prices/dates, no false-intimacy phrases on first touch, deferrals instead of fabricated commitments). See [[features/outreach.md]].

## Outreach & CRM P1 foundation shipped

**Date:** 2026-04-19
Added the first slice of the CRM/outreach layer: a new top-level `internal/outreach/` module with channel-abstraction, contacts, sender profiles, and Gmail OAuth (auth URL + callback + encrypted token storage + send via Gmail API + inbox polling). Migrations 8-9 add `contacts`, `user_channels`, `sender_profiles`, `conversations`, `messages` tables. Channel interface is designed so WhatsApp/LinkedIn drop in without touching conversation/campaign code. See [[features/outreach.md]] and [[decisions.md#adopt-channel-interface-for-pluggable-messaging-transports]].

## Excel-only discovery redesign shipped

**Date:** 2026-04-18
The original 4-stage discovery wizard (product name → HS detection → country ranking → enrichment) was collapsed to a single Excel upload. All derivation now happens deterministically from the Excel itself, with one grounded AI call per market for the product name. See [[decisions.md#collapse-4-stage-discovery-into-excel-only-upload]]. Deleted prompts `hsdetect.go`, `marketsummary.go`, `analyze.go`, `queries.go`. Added `product_name.go`. Migration 7 (`000007_market_derived_metadata`) extended the `markets` table with derived-metadata columns.

## Cloud Run configured for async background work

**Date:** 2026-04-18
Set `--no-cpu-throttling` + `--timeout=1800` + `--min-instances=0` on the backend service. Tried `min-instances=1` briefly (would have blown free tier) and reverted. Current setup: instance spins up on request, stays alive with CPU while enrichment goroutines run, scales to zero when idle. See [[decisions.md#cloud-run-with-cpu-always-allocated-min-instances0-30-min-timeout]].

## Bug: UPLOAD failed due to varchar(3) on UNKNOWN country code

**Date:** 2026-04-18
First deploys crashed with `SQLSTATE 22001: value too long for type character varying(3)` when an Excel had a ConsigneeCountry my parser couldn't map. Fixed by changing the fallback from `"UNKNOWN"` to `"ZZ"` and skipping the group entirely with a warning. See [[bugs.md#unknown-country-fallback-overflowed-varchar3]].

## Published repo to GitHub

**Date:** 2026-04-18
`batucodein/marketintel` (private). Created 7 V2 issues for deferred features (multi-origin support, merge duplicate uploads, re-enable market analysis when trade data is real, HS-code splitting, Firecrawl enrichment, cross-discovery intelligence, email marketing workflow).

## Initial cloud deploy

**Date:** 2026-04-13
Backend + frontend deployed to Cloud Run (`marketintel-493113`), Supabase for Postgres, Firebase for... nothing as it turned out (we kept everything in Cloud Run). Added rate limiting (100/min global, 10/min on /auth/*).

## Website enrichment upgraded

**Date:** 2026-04-10
Added `website_data` JSONB column (migration 6), expanded the scraper to extract multiple emails, phone numbers, WhatsApp links, social media, contact forms, and product keywords. Added SMTP-level email verification.

## Shipment data column refactor

**Date:** 2026-04-09
Moved shipment/customs data out of `social_links` (which was a misnomer) into a dedicated `shipment_data` JSONB column (migration 5). Data was flattened — previously wrapped in `{"shipment_data": {...}}`, now stored directly.

## AI cost tracking per discovery

**Date:** 2026-04-07
Added `search_id` foreign key on `ai_request_log` (migration 4). Dashboard now shows cost-per-discovery in addition to daily totals.

## Go rewrite

**Date:** 2026-04-04
Replaced the original FastAPI + Celery Python backend with a Go monolith. Discovery pipeline became goroutine-based instead of task-queue-based. See [[decisions.md#go-backend-over-python-backend-before-repo-initialization]].

## Project started

**Date:** 2026-03-28
First commits. Initial schema migration (`000001_initial_schema.up.sql`) created users, product_categories, businesses, trade_flows, markets, business_markets, lead_scores, searches.
