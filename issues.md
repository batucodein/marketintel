# Known Issues — to discuss / fix later

Running log of identified-but-not-yet-fixed issues. Each entry has enough context to pick up cold.

---

## 1. Inbound "from scratch" email is dropped (thread-ID-only matching) — compliance risk

**Severity:** High (engagement loss + CAN-SPAM/GDPR opt-out hole)
**Status:** FIXED (code; pending deploy) — addressed 2026-06-15 as part of the native IMAP/SMTP email-connectivity build. `poller.persistIncoming` now uses a matching ladder: thread-id → In-Reply-To (FindByExternalID) → from-address (FindActiveByContactEmail) → else ignore. IMAP (no thread id) needed this, and it closes the from-scratch / from-scratch-unsubscribe gap. Verify in the live E2E.
**Found:** 2026-06-14

### What happens
The inbound poller matches a received email to a conversation **only by thread ID**:

```go
// internal/outreach/poller/poller.go  (persistIncoming)
conv, err := p.conversations.GetByThread(ctx, uc.ID, m.ExternalThreadID)
if err != nil || conv == nil {
    return false   // "Not a reply to something we started — ignore for now."
}
```

If the buyer composes a **new email from scratch** (not an in-thread reply), `ExternalThreadID` doesn't match any conversation we started, so the message is **dropped and never stored**. The same happens if their mail client breaks threading (missing/altered `In-Reply-To`/`References`, changed subject).

### Consequences
1. **Engagement lost** — we never see the reply; the cadence treats them as *no-reply* and keeps sending follow-ups to someone who actually responded.
2. **Compliance hole (the serious one)** — a from-scratch "stop / remove me / unsubscribe" is dropped → the opt-out is never processed → we keep emailing them. Bypasses the entire opt-out path hardened in the audit.

### Why
Matching is thread-ID-only. The inbound message already carries the data for fallback matching but the poller ignores it:
- `channel.IncomingMessage.From` — sender's email (from the From header, populated by the Gmail adapter)
- `channel.IncomingMessage.InReplyToExternalID` — the In-Reply-To header

### Proposed fix — matching ladder
1. **Thread ID** (current — exact, keep first).
2. **In-Reply-To** → find OUR sent message whose `external_id` == `InReplyToExternalID` (reuse `conversations.FindByExternalID`) → use its `ConversationID`. Catches replies whose client broke threading.
3. **From address** → parse `Name <email>` to the bare address → find the contact by email (scoped to the channel's user) → attach to that contact's most-recent **active** conversation on this channel. Catches the true "from scratch" case.
4. Else → still ignore (a genuine stranger isn't our outreach).

**Guardrails:** parse `Name <email>` to bare address; scope the from-match to the channel's user AND an existing contact already in an active conversation (never attach a random stranger); pick the most-recent conversation when several exist; keep existing idempotency (`FindByExternalID`) and the classify→persist→flip ordering; route the matched message through the same opt-out / sentiment / branch path so a from-scratch unsubscribe is honored.

### Files
- `internal/outreach/poller/poller.go` (`persistIncoming` — add the fallback ladder)
- `internal/outreach/conversation/repository.go` (may need: find contact by email + latest active conversation by contact; `FindByExternalID` already exists for the In-Reply-To step)
- `internal/outreach/channel/gmail/gmail.go` (From/In-Reply-To already populated — no change expected)

---

## 2. `UpdateBusinessContact` is unscoped (IDOR) — any user can edit any business's contact info

**Severity:** Medium (cross-tenant write; data integrity)
**Status:** Open — deferred during the 2026-06-14 market-scoping fix
**Found:** 2026-06-14 (while fixing the markets cross-tenant leak)

### What happens
`PATCH /markets/{marketID}/leads/{businessID}` → `UpdateBusinessContact(ctx, businessID, email, phone, website)` updates a business row with **no ownership check** — any authenticated user can edit any business's email/phone/website by guessing/knowing a `businessID`.

```go
// internal/scoring/handler.go (UpdateLead) → repository.UpdateBusinessContact — no user scoping
```

### Why it was deferred
Businesses aren't directly user-owned (no `user_id` on `businesses`). Correct scoping is via the user's relationship to the business — e.g. an owned `market` + `business_markets`, or the user's `lead_scores` for that business. More involved than the market fix, so left out to keep that change tight.

### Proposed fix
Gate the update on the acting user owning a market the business belongs to (join `business_markets` → `markets` → the user's `searches`), or owning a `lead_scores` row for `(business_id, user_id)`. Return `ErrNotFound` otherwise. Mirror the market ownership pattern (`UserOwnsMarket`).

### Files
- `internal/scoring/handler.go` (`UpdateLead`)
- `internal/scoring/repository.go` (`UpdateBusinessContact` — add ownership-scoped predicate)

### Related
Same tenant-scoping class as the markets leak fixed 2026-06-14 (see project memory invariant). Markets now scope via `searches.user_id`; businesses need an analogous anchor.
