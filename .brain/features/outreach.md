---
type: feature
updated: 2026-04-19
---

# Outreach & CRM

## Overview

The outreach layer lets users email their discovered leads from inside MarketIntel, see replies as chat threads, and (future) run multi-step campaigns with AI-drafted follow-ups. Designed channel-agnostic from day one — Gmail today, WhatsApp/Outlook/SMTP adapters slot in later without touching conversation or campaign code.

The full vision (discovery → score → email → reply → follow-up → close, in-app) is planned across P1-P5 phases documented in the plan file.

## Phase status

- **P1 (in progress, 2026-04-19)** — Foundation: Gmail OAuth, contacts, sender profile, channels, conversation view. Backend scaffolding mostly complete; conversation service handlers, AI draft prompts, polling worker, and full frontend still to do.
- **P2** (not started) — Campaigns (bulk outreach)
- **P3** (not started) — Sequences (automated follow-ups)
- **P4** (not started) — CRM extras (tasks, notes, pipeline stages)
- **P5** (not started) — Team collaboration

## Architecture highlights

- **Contact-centric data model.** One `contacts` row per `(user_id, business_id)`. A company appearing in 3 different uploaded markets still maps to ONE contact the user has a relationship with. See [[decisions.md#one-contact-per-user-business-crm-is-contact-centric-not-market-scoped]].
- **Channel interface.** `outreach/channel/interface.go::Channel` — every transport (Gmail now, WhatsApp/Outlook later) implements `Send` and `ListNewMessages`. Conversation/campaign code depends only on the interface. See [[decisions.md#adopt-channel-interface-for-pluggable-messaging-transports]] and [[patterns.md#pluggable-messaging-transports-via-a-channel-interface]].
- **Gmail via API, not SMTP.** Sending goes through `users.messages.send` so messages appear in the user's real Sent folder. See [[decisions.md#use-gmail-api-usersmessagessend-not-smtp-relay]].
- **OAuth callback is public.** Google has no way to present our JWT, so the callback route is registered outside the auth group. User identity is carried in the OAuth `state` parameter. See [[decisions.md#gmail-oauth-callback-is-a-public-route-not-behind-auth-middleware]] and [[patterns.md#public-oauth-callback-sits-above-the-authenticated-mount]].
- **Encrypted tokens at rest.** AES-256-GCM via `platform/crypto`. See [[decisions.md#aes-256-gcm-encryption-for-oauth-tokens-at-rest-derived-from-secret_key-in-dev]].

## Key files (backend)

- `migrations/000008_crm_core_tables.up.sql` — contacts, user_channels, sender_profiles
- `migrations/000009_outreach_conversations.up.sql` — conversations, messages
- `internal/domain/outreach.go` — Contact, UserChannel, SenderProfile, Conversation, Message
- `internal/outreach/handler.go` — top-level router at `/outreach`
- `internal/outreach/channel/interface.go` — the pluggable transport contract
- `internal/outreach/channel/registry.go` — factory lookup by channel-type
- `internal/outreach/channel/repository.go` — user_channels DB access with encrypted tokens
- `internal/outreach/channel/handler.go` — `/outreach/channels/*` + Gmail OAuth URL/callback
- `internal/outreach/channel/gmail/gmail.go` — Channel implementation for Gmail
- `internal/outreach/sender/` — sender profile CRUD
- `internal/outreach/contact/` — contact upsert + list
- `internal/platform/oauth/gmail.go` — OAuth2 config, token refresh, userinfo fetch
- `internal/platform/crypto/aes.go` — AES-256-GCM helper

## HTTP surface (P1 backend complete)

Authenticated under `/outreach`:
- `GET/PUT /outreach/sender-profile` — user's positioning for AI
- `GET /outreach/contacts`, `GET /outreach/contacts/{id}`, `PATCH /outreach/contacts/{id}`
- `GET /outreach/channels` — list connected Gmail accounts
- `DELETE /outreach/channels/{id}` — disconnect
- `POST /outreach/channels/{id}/default` — mark default
- `GET /outreach/channels/gmail/auth-url` — get URL for the consent redirect
- `GET /outreach/conversations` — inbox (filters: unread, pagination)
- `POST /outreach/conversations` — start a new thread with a contact (optional AI draft)
- `GET /outreach/conversations/{id}` — thread detail + messages
- `PATCH /outreach/conversations/{id}` — update status or automation
- `POST /outreach/conversations/{id}/read` — mark read
- `POST /outreach/conversations/{id}/messages` — send a message (draft_message_id OR body+subject)
- `POST /outreach/conversations/{id}/draft` — AI-suggested reply

Public (no auth):
- `GET /outreach/channels/gmail/callback` — Google's redirect target

## Background worker

`internal/outreach/poller/poller.go` runs on a 2-minute tick, iterates every `enabled=true` row in `user_channels`, calls `Channel.ListNewMessages(since)`, matches each by Gmail `threadId` to an existing conversation, inserts inbound messages and marks the conversation unread. Sees no messages → updates `last_poll_at` and moves on. Spawned from main.go at boot.

## Timeline

- **2026-04-19** — P1 backend complete: conversations, AI draft/reply prompts, Gmail inbox poller. End-to-end flow works. See [[history.md#outreach-p1-backend-complete-end-to-end-gmail-flow-works]].
- **2026-04-19** — P1 foundation: migrations, domain types, channel interface, Gmail OAuth plumbing, Gmail API send/fetch, sender profile, contact upsert. See [[history.md#outreach-crm-p1-foundation-shipped]].

## Current state

**Backend: P1 complete.** Fully operational end-to-end. Gmail OAuth working both locally and on Cloud Run. The only thing blocking real usage is the frontend.

**Frontend: not started.** Needed to ship P1 to users: outreach layout (sub-sidebar), setup wizard page, sender profile form, connected channels page, inbox view, conversation chat view, "Email this lead" button on existing lead list page.

**Blockers for end-to-end test (beyond the frontend):**
- None for the logged-in user path — all backend endpoints return 200/202 on valid input.
- For a UI-less test: use curl with a valid JWT to `POST /outreach/conversations` after connecting Gmail via `/outreach/channels/gmail/auth-url`.
