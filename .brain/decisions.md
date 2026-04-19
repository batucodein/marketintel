---
type: decisions
updated: 2026-04-19
---

# Decisions

## Outreach AI guardrails: no invented prices, no false-intimacy phrases, deferrals over fabrication

**Date:** 2026-04-19
**Status:** Active
**Context:** Generic anti-hallucination preamble isn't enough for outreach. The model has a natural pull toward sounding impressive — it will invent prices, pretend there was a prior meeting, quote volumes that weren't in the data. Any of these shipped to a real prospect damages our user's reputation.
**Alternatives considered:**
- Rely on the shared `NoHallucinationPreamble` (too generic — doesn't catch outreach-specific pitfalls)
- Require user to review every AI draft (we do — but the draft should not contain hallucinations in the first place)
- Outreach-specific prompt rules + post-generation regex scan (chosen for V2 scan; V1 prompt rules alone)
**Result:** `outreach_draft.go` and `outreach_reply.go` prompts enforce: (1) never include specific prices/discounts/numeric offers unless sender profile provided them; use "happy to share on request" style phrasing instead. (2) never imply a prior conversation — first touch is explicitly labeled. (3) for reply-time questions the AI can't answer from provided data, defer ("I'll send a rate sheet shortly") or ask clarifying questions. (4) `needs_user_input` field on replies lists any facts the user should verify before sending. Post-generation regex scan (reject-and-retry on matches like `$\d+`, "as we discussed") tracked for V2.

## Inbox polling: matchbythreadId, drop stranger inbound messages for P1

**Date:** 2026-04-19
**Status:** Active
**Context:** The Gmail inbox polling worker pulls all new messages every 2 minutes. Some of those are replies to our outbound; some are unrelated emails the user received from strangers.
**Alternatives considered:**
- Auto-create a contact + conversation for every inbound sender (adds clutter; most inbound is noise)
- Drop anything we can't match to an existing conversation (chosen for P1)
- Surface unmatched inbound in a separate "unassigned" queue (deferred to P4)
**Result:** `poller.persistIncoming` only stores inbound messages whose Gmail `threadId` matches an existing conversation in our DB. Everything else is silently skipped. This keeps the inbox strictly focused on threads the user started through our tool. Tracked as a V2 concern: if the user proactively cold-emails from Gmail and the recipient replies, we won't pick it up.

## Adopt channel interface for pluggable messaging transports

**Date:** 2026-04-19
**Status:** Active
**Context:** Outreach will start with Gmail, but product roadmap includes WhatsApp, Microsoft/Outlook, SMTP fallback, LinkedIn. If we bind the conversation/campaign/sequence code directly to Gmail APIs, every future channel becomes a refactor.
**Alternatives considered:**
- Gmail-only for V1, refactor later when needed (rejected — refactoring a live system with user data is expensive; better to pay the small abstraction tax upfront)
- Use an off-the-shelf abstraction like Nylas (SaaS, adds dependency + cost + latency)
- Channel interface + registry in our own code (chosen)
**Result:** `internal/outreach/channel/interface.go` defines `Channel` (Type, CanSend, Send, ListNewMessages). `registry.go` maps channel-type strings to factories. Today only `gmail_oauth` is registered; adding WhatsApp is a new package that implements the interface and a single `DefaultRegistry.Register` call. Conversation/campaign/sequence code calls only `channel.Channel`, never touches Gmail directly.

## One contact per (user, business) — CRM is contact-centric, not market-scoped

**Date:** 2026-04-19
**Status:** Active
**Context:** A company often appears in multiple markets the user has uploaded (e.g., ACME imports marble AND granite AND travertine from 3 different Excels). If we key outreach to market-lead, the same company creates three separate lead "identities", three separate conversations, and a risk of double-outreach.
**Alternatives considered:**
- Market-scoped leads (simpler queries, bad UX — same company split across "cards")
- Contact per (user, business) — one record across all markets (chosen — this is how every serious CRM works: HubSpot, Pipedrive, Salesforce)
**Result:** `contacts` table has `UNIQUE (user_id, business_id)`. On "Email this lead" or add-to-campaign, we upsert the contact lazily. Markets still reference businesses in `business_markets`, unchanged. The outreach layer joins through contacts. Campaign wizard skips contacts already in active conversations by default with a warning.

## Gmail OAuth only for V1 outreach — SMTP and Outlook deferred

**Date:** 2026-04-19
**Status:** Active
**Context:** Need to pick an email transport to ship V1. Google Workspace + personal Gmail users are the biggest segment among Turkish B2B exporters; other providers (Outlook, Zoho, cPanel SMTP) add complexity.
**Alternatives considered:**
- Gmail + Outlook OAuth + SMTP fallback simultaneously (most coverage, ~2x work)
- Transactional service (Resend/Postmark) — sends from our domain, requires DNS setup per user, less "personal" feel
- Gmail OAuth only (chosen for V1)
**Result:** `platform/oauth/gmail.go` + `outreach/channel/gmail/` package. Users not on Google Workspace can't use the tool in V1. Microsoft OAuth and SMTP fallback tracked as V2 tasks. Channel interface designed so adding them is ~1 day each.

## Use Gmail API (users.messages.send), not SMTP relay

**Date:** 2026-04-19
**Status:** Active
**Context:** Even with Gmail OAuth we could send via SMTP with the access token. But then sent messages would not appear in the user's Gmail Sent folder.
**Alternatives considered:**
- SMTP via Google's relay (simpler code, worse UX)
- Gmail API users.messages.send (chosen)
**Result:** When a user sends via our tool, the message appears in their real Gmail Sent folder. They see the same conversation in Gmail and in our inbox. If they reply in Gmail directly, our polling worker pulls it in. Seamless.

## Gmail OAuth callback is a public route (not behind auth middleware)

**Date:** 2026-04-19
**Status:** Active
**Context:** Google's redirect to our callback endpoint has no Authorization header — we can't require our JWT. But most of our API is behind `RequireAuth` middleware, including everything mounted under `/outreach`.
**Alternatives considered:**
- Move the callback to a completely different path like `/oauth-callbacks/gmail` (works but clutters the URL space)
- Use two Mount points at `/outreach` (chi rejects overlapping mounts)
- Register one specific `Get("/outreach/channels/gmail/callback", ...)` on the root router BEFORE the authenticated `Mount("/outreach", ...)` group (chosen)
**Result:** In `server.go`, the callback route is registered publicly first; chi's trie matches the specific path before the Mount subtree, so the callback bypasses auth. User identification inside the callback uses the OAuth `state` parameter (short-lived in-memory CSRF store, TTL 10 min).

## AES-256-GCM encryption for OAuth tokens at rest, derived from SECRET_KEY in dev

**Date:** 2026-04-19
**Status:** Active
**Context:** OAuth refresh tokens give perpetual access to the user's Gmail — they must be encrypted at rest. But requiring a dedicated `TOKEN_ENCRYPTION_KEY` in local dev is friction.
**Alternatives considered:**
- Always require a separate key (safest, worst DX)
- Store plaintext in dev (common lazy choice; leaks into accidental prod deploys)
- Prefer explicit `TOKEN_ENCRYPTION_KEY`; if missing, derive from `SECRET_KEY` via SHA-256 (chosen)
**Result:** `platform/crypto/aes.go` exposes `NewFromKeyOrSecret`. In production we set `TOKEN_ENCRYPTION_KEY=$(openssl rand -base64 32)`. In dev the key is deterministic from `SECRET_KEY`, so tokens remain readable across restarts but the mechanism is identical. Rotation story: if the key changes, existing tokens become unreadable and users must re-connect — acceptable for now.

## Collapse 4-stage discovery into Excel-only upload

**Date:** 2026-04-18
**Status:** Active
**Context:** The original flow had four stages: product-name input → AI HS-code detection → Comtrade market ranking → Excel/API import → lead scoring. Too many steps, and two of them (HS detection, market ranking) were inherently hallucination-prone because the AI had to infer facts from a free-text product name with no grounding.
**Alternatives considered:**
- Keep the 4-stage flow and add stricter hallucination caps (rejected — treats symptom, not cause)
- Replace HS detection with a user-picked dropdown (rejected — still forces user input and adds UX friction)
- Excel-only (chosen) — every piece of metadata (HS code, product, origin country, date range) is derivable from the customs data itself
**Result:** Dropped handlers `CreateDiscover`, `RankMarkets`, `ExploreCountry`, `ImportExcel`, `GenerateLeads`, `GetSources`. Dropped prompts `hsdetect.go`, `marketsummary.go`, `analyze.go`, `queries.go`. New single entry point `POST /discover/` accepts only a file.

## Split multi-country Excels into multiple markets

**Date:** 2026-04-18
**Status:** Active
**Context:** A customs Excel can contain shipments to several destination countries (US + Canada + Mexico). The question was whether to force the user to upload one file per country, auto-merge into a single market, or split.
**Alternatives considered:**
- Majority wins, one market (simpler UX, but loses data for minority countries)
- Reject mixed files (pushes cleanup to the user)
- Split into N markets (chosen — most accurate, and the frontend already handles multiple market cards well)
**Result:** `SplitByDestination(records)` in `internal/discovery/market_metadata.go` buckets by ConsigneeCountry (normalized to ISO-2). Each bucket becomes its own market with its own enrichment goroutine. One upload can produce N scored lead lists in parallel.

## Delete market analysis feature (analyze.go)

**Date:** 2026-04-18
**Status:** Active
**Context:** The `/markets/{id}/analysis` endpoint used AI to generate market size / demand / pricing / competitive analysis. Without a real trade-data source (Comtrade was removed), the AI had no grounding — it would invent market size and pricing. Exactly the hallucination risk the project can't tolerate.
**Alternatives considered:**
- Keep with `data_completeness` cap (flags low confidence but doesn't prevent invented numbers)
- Integrate a paid trade-data source so AI has real grounding (defer to v2 — out of scope now)
- Remove entirely (chosen)
**Result:** Endpoint + prompt + frontend analysis card all deleted. `market_analyses` table rows cleared in migration 7 (table itself kept for now). Tracked as GitHub issue #3 to revisit when a real trade-data source is available.

## Use AI for grounded summarization, not invention

**Date:** 2026-04-18
**Status:** Active
**Context:** Tension between "zero hallucination" and "we want nicer product names on market cards." Initial thinking was to avoid AI entirely and use raw Excel text or HS-code descriptions. User pushed back: AI summarizing real data is not hallucination.
**Alternatives considered:**
- HS-code description only (clean but generic — "Marble, travertine, alabaster")
- Raw Excel product text (noisy — "MARBLE SLABS POLISHED 2CM...")
- AI-summarized from real product descriptions (chosen)
**Result:** `product_name.go` prompt takes the top 20 product descriptions from Excel as grounding and returns a clean name like "Marble sinks and vanity tops". Includes mandatory `data_completeness` and an origin-singleness sanity check. This clarified the rule for the whole project: AI is fine when summarizing provided data, not when inventing.

## Remove home-country input and buys-from-home trust tier

**Date:** 2026-04-18
**Status:** Active
**Context:** Original design tagged importers as `confirmed_buyer` vs `confirmed_importer` based on whether any of their suppliers were from the exporter's home country. Required the user to input their home country on every upload.
**Alternatives considered:**
- Pre-fill from user profile, editable per upload (still adds form complexity)
- Always require (friction)
- Remove (chosen — the UX simplification outweighed the tier signal)
**Result:** All importers now tagged `confirmed_importer`. `buys_from_home` logic gone from the pipeline. Parser still accepts a `homeCountry` string for backward compat but the new `ProcessExcel` path passes `""`.

## Keep multiple HS codes in one market instead of splitting

**Date:** 2026-04-18
**Status:** Active
**Context:** A marble Excel might contain 6802.91 (marble) + 6802.93 (granite) + 2515.12 (raw blocks). Should one upload with 3 HS codes produce 3 markets?
**Alternatives considered:**
- Split per HS code (most accurate, UX explosion — one upload → N*M markets for N countries × M HS codes)
- Show all HS codes inline on card (cluttered)
- Dominant HS code + "+N related" badge on card (chosen)
**Result:** The dominant 6-digit HS code by frequency is stored as `dominant_hs_code`; the full list as `all_hs_codes TEXT[]`. Market card shows the dominant one with a "+N related" badge. Classification and scoring still see all HS codes per business via `shipment_data`.

## Use 5th–95th percentile for market date range

**Date:** 2026-04-18
**Status:** Active
**Context:** Customs data often has outliers (1 shipment from 2019 in an otherwise-2025-only file). Raw min-max is misleading.
**Alternatives considered:**
- Raw min–max (simple, wrong)
- Last-12-months rolling (predictable, hides real history)
- 5th–95th percentile (chosen)
**Result:** `DeriveMarketMetadata` sorts arrival dates and picks the 5% and 95% positions. Where 90% of shipments actually happened. Market card renders `Mar 2026 – Mar 2026 (1 month)`.

## Keep separate markets on duplicate uploads instead of merging

**Date:** 2026-04-18
**Status:** Active
**Context:** If the user uploads "marble → US" in January and again in April, do these merge into one ever-growing market or stay as two snapshots?
**Alternatives considered:**
- Merge if HS + destination match (cleaner list, more logic, harder to compare snapshots)
- Keep separate (chosen — current behavior: `CreateMarket` always INSERTs)
**Result:** Each upload is a snapshot in time. Users can compare January's lead list against April's. Trade-off: the markets list will show duplicates when uploading the same data periodically. Tracked as GitHub issue #2 for v2.

## Rate-limit auth endpoints more strictly than rest of API

**Date:** 2026-04-13
**Status:** Active
**Context:** After initial cloud deploy we needed basic brute-force protection without adding a WAF.
**Alternatives considered:**
- Cloud Armor WAF (expensive, overkill at our scale)
- In-app with httprate (chosen)
**Result:** Global 100 req/min/IP, auth routes scoped to 10 req/min/IP. Implemented as middleware in `internal/server/server.go`.

## Cloud Run with CPU always allocated, min-instances=0, 30-min timeout

**Date:** 2026-04-18
**Status:** Active
**Context:** Background enrichment goroutines need CPU after the HTTP handler returns. Without "CPU always allocated", Cloud Run throttles the instance to zero CPU and goroutines die. With `min-instances=1` the instance runs 24/7 and blows the free tier (~$60/month after trial).
**Alternatives considered:**
- min-instances=1 (expensive)
- Move to Cloud Tasks for durable background jobs (overkill at current scale, needs more infra)
- CPU always allocated + min-instances=0 (chosen)
**Result:** `gcloud run services update marketintel --no-cpu-throttling --min-instances=0 --timeout=1800`. Frontend polling every 3s keeps the instance warm during enrichment. When no uploads run, instance scales to zero and costs nothing. Trade-off: if the user closes the tab mid-enrichment and the instance scales down before completion, goroutines die. Acceptable for current scale.

## Deploy to Cloud Run + Supabase instead of Neon + Firebase Hosting

**Date:** 2026-04-12
**Status:** Active
**Context:** Needed a free-tier deploy target. Existing poker app already used Cloud Run + Firebase Hosting + Neon.
**Alternatives considered:**
- Neon (database only — same tier already used for poker, was nearing 500 MB limit when combined)
- Supabase (fresh 500 MB bucket for marketintel — chosen)
- Cloud SQL (burns more GCP credits)
**Result:** GCP project `marketintel-493113`, Supabase for Postgres, Cloud Run for both backend and frontend containers. Full setup in `DEPLOYMENT.md` (gitignored).

## Go backend over Python backend (before repo initialization)

**Date:** 2026-04-04
**Status:** Active
**Context:** Project started as a Python (FastAPI + Celery) stack but was rewritten to Go before the current repo was initialized.
**Alternatives considered:**
- FastAPI + SQLAlchemy + Celery workers (original)
- Go monolith with goroutines (chosen — simpler ops, better type safety, fewer runtime surprises for per-market async work)
**Result:** Current `backend-go/` structure. Python tree no longer in repo. The decision to use goroutines for async work (instead of a task queue) is what makes the current Cloud Run CPU-always-allocated setup necessary.
