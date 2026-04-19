# .brain/ Schema v2.0

The `.brain/` directory is a per-repo project memory tracked in git. It gives any LLM coding tool (Claude Code, Cursor, Codex, Copilot) full project context at session start.

---

## LLM Instructions

If you are an LLM reading this file, follow these instructions. This section tells you how to read, maintain, and update this project's brain. No additional tools or skills are required — everything you need is in this file.

### At Session Start

1. Read `.brain/index.md` to understand the project.
2. Based on the user's task, read relevant pages:
   - Refactoring or adding code → `architecture.md`, `patterns.md`
   - Debugging → `bugs.md`, relevant `features/*.md`
   - New feature → `decisions.md`, related `features/*.md`
   - General question → search across all pages
3. If the task relates to a specific feature, check if `features/X.md` exists and read it for full lifecycle context.
4. **If the user's question touches events older than the active pages' date range** (dates preceding the oldest entry in the relevant active page, or mentions of multi-year-old work), also check `.brain/archive/*.md` before answering. Archive is cold storage — loaded on demand, not at session start. Under-reading archive gives incomplete answers; over-reading is harmless.

### During the Session — Track Reasoning

The conversation is the only place where reasoning exists. Recognize these moments as they happen — they are the WHY that brain captures:

1. **A choice was made** — Multiple approaches existed, one was picked. Note what was chosen and what wasn't.
2. **Something broke and was understood** — A bug was reported, investigated, and diagnosed. The full arc matters: symptom → investigation → root cause → fix.
3. **Something was rejected** — An approach was proposed and turned down, or was tried and didn't work. Why it was rejected is high-value context.
4. **A constraint shaped the work** — Performance, cost, compatibility, time, team size — anything that steered the implementation away from the default approach.
5. **The system changed structurally** — New component, integration, dependency, or layer. Structural reasoning is the hardest to recover later.

You don't need to write anything during the session — just recognize these moments so you can reference them when updating brain.

**Proactive flush when the session grows:** after 3+ substantive events of the types above have accumulated in this session without a brain update, proactively offer `/brain update` to the user. Session context can be compacted or lost; partial WHY captured now beats complete WHY lost later.

### Updating .brain/ — The Process

After significant code changes (or when asked to update brain), follow this process:

#### 1. Get the WHAT (code changes)

Check for uncommitted changes AND recent commits since the last brain update:

```bash
# Uncommitted changes (always check)
git diff HEAD -- ':(exclude)*.lock' ':(exclude)package-lock.json' ':(exclude)go.sum' ':(exclude).brain/' 2>/dev/null | head -300
git diff --cached -- ':(exclude)*.lock' ':(exclude)package-lock.json' ':(exclude)go.sum' ':(exclude).brain/' 2>/dev/null | head -300

# Commits since last brain update (only if brain has been committed before)
LAST_BRAIN=$(git log -1 --format=%ci -- .brain/ 2>/dev/null)
if [ -n "$LAST_BRAIN" ]; then
  git log --since="$LAST_BRAIN" --oneline --stat -- ':(exclude).brain/' 2>/dev/null
fi
```

#### 2. Filter for significance

Skip the update if ALL changes are:
- Formatting, linting, or whitespace only
- Dependency version bumps with no behavior change
- Renaming with no design reasoning
- Changes to fewer than 3 lines with no architectural impact

Continue only if at least one change involves a new feature, bug fix, architectural change, explicit decision, pattern change, or infrastructure change.

#### 3. Categorize changes

| Category | Brain pages to update |
|----------|----------------------|
| **New feature** | `history.md`, create `features/X.md`, maybe `architecture.md` |
| **Bug fix** | `bugs.md`, update `features/X.md` if related |
| **Decision** | `decisions.md`, link from `features/X.md` if related |
| **Pattern change** | `patterns.md` |
| **Stack/dependency change** | `index.md` |
| **Architecture change** | `architecture.md` |
| **Refactor** | `history.md`, `architecture.md` if structure changed |

**When the category isn't obvious:** if the change could plausibly be two of {new feature, decision, pattern change, architecture change}, pick the best fit and note the alternative one line in the entry body (e.g., `Also has pattern-change aspects — kept as decision because the choice among alternatives was the driver.`). Do NOT stall on minor ambiguity. Ask the user only when you truly cannot pick.

**Examples of reasoning (not rules):**
- `Switching ORMs` → **decision**. A reversible choice among alternatives; capture what was weighed.
- `Moved auth middleware from request-scoped to session-scoped` → **architecture change**. Changed where it runs in the stack, not just how it's written.

#### 4. Extract the WHY from conversation

For each categorized change, look back through the conversation for the 5 event types listed in "During the Session" above. Match changes to reasoning.

**If the WHY is partial** (conversation has some signal but not the full rationale): capture what you have and explicitly mark the gap:
`**Context:** [partial — speed was the driver; alternatives weren't discussed in session.]`
This does NOT violate Rule #1 (never hallucinate) — invented WHY does; *documented partial* WHY does not. Partial beats placeholder.

**If no WHY exists at all** in the conversation: Do NOT invent one. Either:
- Ask the user what drove the change
- Or write the entry with just the WHAT and mark it: `**Context:** To be filled — reasoning not captured in session.`

#### 5. Write updates

For each page that needs updating:
1. Read the current content.
2. Verify frontmatter is intact: `type:` and `updated:` fields must exist. If missing, add them.
3. Update the `updated:` frontmatter field to today's date (YYYY-MM-DD) before writing content.
4. Add new entries at the top (newest first) for `history.md`, `decisions.md`, `bugs.md`.
5. Replace in-place for `index.md`, `architecture.md`, `patterns.md`.
6. Follow the format rules in this file. Every entry needs `**Date:** YYYY-MM-DD`.
7. Add `[[wikilinks]]` to connect related entries across pages.
8. If a significant new feature was built and no `features/X.md` exists, create one.

**Writing rules:**
- Lead with the WHY, not the WHAT. Bad: "Added Redis caching." Good: "Added Redis caching to avoid redundant API calls — same queries were hitting the API repeatedly, costing money and adding latency."
- Include alternatives considered when available.
- For bugs, always include root cause and lesson.
- Keep entries to 1-3 short paragraphs. Brain is context, not documentation.

#### 6. Maintain topic page Timelines

After writing updates to event-type pages (decisions/bugs/history/features), check for existing topic pages and append matching Timeline entries:

1. **List existing topics:** `ls .brain/topics/*.md 2>/dev/null`. If none, skip this step.
2. **For each topic**, read its `## Overview` section to understand the topic's scope. **If the Overview is abstract** (e.g., "caching strategy", "data access patterns") OR the new entry uses terms not in the Overview, ALSO read the topic's 3 most recent Timeline bullets to see what events this topic typically absorbs.
3. **For each new entry written in this session**, match against every topic's scope:
   - Keyword match on the topic's name, slug, or obvious synonyms in the new entry's header/body.
   - If a match is found, construct the proposed Timeline bullet:
     ```
     - **YYYY-MM-DD** — <caption> [[<page>.md#<anchor>]]
     ```
     where `<anchor>` is the slug of the new `##` header you just wrote, per SCHEMA.md § Anchor Slug Algorithm.
   - **Wikilink target disambiguation.** If multiple `##` anchors on the target page share keywords with the new entry, prefer the newest (highest `**Date:**`) unless the new entry's narrative explicitly references an earlier one.
   - **Write-time wikilink validation.** Before appending the bullet, verify the wikilink resolves:
     - Does `<page>.md` exist? (You just wrote it this session, so it should — but confirm for robustness.)
     - Does `<page>.md` contain a `## ` or `### ` header whose slug equals `<anchor>`? Run the slug algorithm from SCHEMA.md over each header and compare.
     - If either check fails: DO NOT append the bullet. Instead, report to the user:
       ```
       Skipped topic maintenance for topics/<slug>.md — could not resolve proposed wikilink
       [[<page>.md#<anchor>]]. The header may have been renamed or the entry wasn't written
       to the expected page. Please check manually.
       ```
     - This prevents accidentally writing a broken wikilink that doctor would only catch later.
   - If validation passes, append the bullet to the topic's `## Timeline` section.
   - After appending, re-sort the topic's full Timeline by `**Date:** YYYY-MM-DD` descending.
   - Update the topic's frontmatter `updated:` to today's date.
4. **Do NOT create new topic pages here.** Topic creation is user-initiated via `/brain topic <name>` only. If this session produced multiple events that seem to warrant a new topic, surface a suggestion:
   > "This session touched `<domain>` in multiple places. Consider running `/brain topic <domain>` to create a synthesis page."

   **Threshold for suggesting a new topic (be strict — weak topics are sticky):**
   - 3+ entries across **multiple sessions** (not just this one) touching the same domain
   - The entries aren't tightly coupled to a single feature (otherwise a `features/<name>.md` is the better home)
   - The domain is likely to recur over time

   If unsure, don't suggest — topics are easier to create later than to delete. Wait for user action; do not create the file.

The wikilink is the authoritative connection between topic and event. The caption is a hint for readers and may drift — doctor enforces wikilink resolution, not caption accuracy.

#### 7. Commit brain updates

**If you haven't committed yet:** Stage brain pages with your code — one commit:

```bash
git add .brain/ src/
git commit -m "feat: add Google OAuth login"
```

**If the code was already committed (e.g., post-commit hook):** Create a separate brain commit:

```bash
git add .brain/
git commit -m "brain: captured OAuth decision and auth architecture change"
```

Brain commits use the `brain:` prefix so they're identifiable in git log.

### Before Session End

If significant changes were made during the session, suggest updating brain. Be specific: "I'd capture the caching decision and the auth bug root cause — want me to update brain?"

### Rules

1. **Never hallucinate.** If you can't determine something from the repo or conversation, write "Unknown — to be filled by team" rather than guessing.
2. **Be concise.** Each entry should be one paragraph or a short list. Brain is context, not documentation.
3. **Explain WHY, not WHAT.** The code shows what. Brain pages explain why.
4. **Use absolute dates.** Always `YYYY-MM-DD`. Never "yesterday" or "last week."
5. **Don't duplicate README.** Reference it: "See README.md for setup instructions."
6. **Respect existing content.** Preserve what others wrote. Add, don't replace (unless correcting errors).
7. **Commit brain updates.** If updating before committing, stage `.brain/` with your code — one commit. If updating after a commit (post-commit hook), create a separate commit with `brain:` prefix.
8. **Compaction.** When any page exceeds 30 entries or 150 lines, move entries older than 3 months to `.brain/archive/<page>-<year>.md`.
9. **Maintain frontmatter.** Every page needs valid frontmatter with `type:` (one of: index, architecture, decisions, patterns, history, bugs, feature, topic, custom, archive) and `updated:` (YYYY-MM-DD). Update `updated:` to today's date whenever you modify a page.

---

## Directory Structure

```
.brain/
├── SCHEMA.md           # This file — format spec + LLM instructions
├── index.md            # Project overview — what, why, who, how
├── architecture.md     # Stack, structure, components, data flow
├── decisions.md        # Why things are the way they are (ADR-lite)
├── patterns.md         # Coding conventions, naming, error handling
├── history.md          # Timeline of significant changes
├── bugs.md             # Notable bugs, root causes, fixes
├── features/           # One page per significant feature (lifecycle tracking)
│   └── *.md
├── topics/             # Cross-cutting narrative synthesis (optional, user-created)
│   └── *.md
├── archive/            # Compacted old entries
│   └── *.md
└── custom/             # Team-defined pages (optional)
    └── *.md
```

## Page Format

Every `.brain/` page is a markdown file with YAML frontmatter:

```markdown
---
type: <page_type>
updated: YYYY-MM-DD
---

# Page Title

Content here. Use markdown normally.
```

### Frontmatter Fields

| Field     | Required | Description                              |
|-----------|----------|------------------------------------------|
| `type`    | yes      | One of the page types below              |
| `updated` | yes      | Date of last meaningful update           |

### Page Types

| Type           | File              | Purpose                                         |
|----------------|-------------------|-------------------------------------------------|
| `index`        | `index.md`        | What this project does, who it's for, tech stack overview |
| `architecture` | `architecture.md` | System structure, key components, data flow, infrastructure |
| `decisions`    | `decisions.md`    | Architectural decisions with context and reasoning |
| `patterns`     | `patterns.md`     | Code conventions, naming rules, error handling, testing approach |
| `history`      | `history.md`      | Timeline of significant milestones and changes  |
| `bugs`         | `bugs.md`         | Notable bugs with root cause and fix             |
| `feature`      | `features/*.md`   | Lifecycle of a significant feature — one page per feature |
| `topic`        | `topics/*.md`     | Cross-cutting narrative synthesizing events from decisions/bugs/history/features — one page per domain |
| `custom`       | `custom/*.md`     | Team-defined pages for domain-specific context   |
| `archive`      | `archive/*.md`    | Compacted old entries from history/decisions/bugs — searchable, not loaded at session start |

## Content Guidelines

### index.md

The entry point. An LLM reads this first to understand the project.

```markdown
---
type: index
updated: 2026-04-11
---

# Project Name

One-paragraph description of what this project does and why it exists.

## Tech Stack

- **Language:** Python 3.12
- **Framework:** FastAPI
- **Database:** PostgreSQL 16
- **Cache:** Redis 7
- **CI/CD:** GitHub Actions

## Key Directories

- `src/api/` — HTTP handlers
- `src/services/` — Business logic
- `src/models/` — Database models
- `src/utils/` — Shared utilities
- `web/` — Frontend (React)

## Team

- 2 backend engineers, 1 frontend
- Project lead owns infra decisions
```

### decisions.md

Each entry is a decision with context. Newest first.

```markdown
---
type: decisions
updated: 2026-04-11
---

# Decisions

## Chose Redis over Memcached for session caching
**Date:** 2026-04-10
**Context:** Needed server-side session store with invalidation support.
**Decision:** Redis — pub/sub enables cache invalidation across instances.
**Alternatives considered:** Memcached (simpler but no pub/sub), database sessions (too slow).
**Status:** Active

## Moved from REST to gRPC for internal services
**Date:** 2026-03-15
**Context:** Inter-service latency was 40ms+ with JSON serialization.
**Decision:** gRPC with protobuf for internal calls. REST remains for public API.
**Alternatives considered:** MessagePack over HTTP (marginal gain), GraphQL (overkill for internal).
**Status:** Active
```

**Status field:** `Active`, `Superseded`, `Deprecated`, `Rejected` (considered but not taken).

**When a decision is reversed or replaced:**

Decisions are append-only. Never edit the prose of a past entry. But when a later decision supersedes an earlier one, add ONE permitted edit to the old entry — extend the Status line:

```markdown
## Chose Redis over Memcached for session caching
**Date:** 2026-04-10
**Context:** Needed server-side session store with invalidation support.
**Decision:** Redis — pub/sub enables cache invalidation across instances.
**Alternatives considered:** Memcached (simpler but no pub/sub), database sessions (too slow).
**Status:** Superseded by [[decisions.md#switched-to-valkey-after-license-change]] (2026-06-15)
```

And add the new decision at the top (newest first) with full context:

```markdown
## Switched to Valkey after license change
**Date:** 2026-06-15
**Context:** Redis 7.4 license change made continued use of Redis incompatible with our redistribution model.
**Decision:** Fork to Valkey (BSD-licensed Redis fork). API-compatible; migration is a rename.
**Alternatives considered:** Dragonfly (commercial), in-house memcached wrapper (too much work).
**Supersedes:** [[decisions.md#chose-redis-over-memcached-for-session-caching]]
**Status:** Active
```

Add a corresponding history.md entry noting the switch. The `**Status:**` edit on the old entry is the ONLY permitted mutation to an already-recorded decision — do not rewrite Context, Decision, or Alternatives. History is authoritative.

### history.md

Chronological log of significant changes. Newest first.

```markdown
---
type: history
updated: 2026-04-11
---

# History

## Added Redis caching layer
**Date:** 2026-04-10
Introduced Redis for session caching and rate limiting. Reduced p95 latency from 120ms to 45ms on authenticated endpoints.

## Migrated CI from CircleCI to GitHub Actions
**Date:** 2026-04-01
Moved all pipelines. Build time dropped from 8min to 3min. Saved $200/mo.

## Initial release v1.0
**Date:** 2026-03-15
Shipped core API with user auth, project CRUD, and webhook integrations.
```

### architecture.md

System structure and component relationships. This page must be built from reading actual code, not guessed from folder names.

```markdown
---
type: architecture
updated: 2026-04-11
---

# Architecture

## Overview

Brief description of the system's architecture style and key design principles.

## System Schema

```
┌─────────────────────────────────────────────────────┐
│                     Client                           │
└──────────────────────┬──────────────────────────────┘
                       │ HTTPS
┌──────────────────────▼──────────────────────────────┐
│                    API Layer                          │
│  ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌─────────┐  │
│  │  Auth   │→│  Rate    │→│ Logging │→│ Handler │  │
│  │Middleware│ │  Limit   │ │         │ │         │  │
│  └─────────┘ └──────────┘ └─────────┘ └────┬────┘  │
└────────────────────────────────────────────┬────────┘
                                             │
┌────────────────────────────────────────────▼────────┐
│                  Service Layer                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │ Service  │  │ Service  │  │  Service          │   │
│  │    A     │  │    B     │  │    C              │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────────────┘   │
└───────┼──────────────┼─────────────┼────────────────┘
        │              │             │
┌───────▼──────────────▼─────────────▼────────────────┐
│              Data / External Layer                    │
│  ┌────────┐ ┌───────┐ ┌──────────┐ ┌────────────┐   │
│  │Database │ │ Cache │ │ External │ │ External   │   │
│  │        │ │       │ │  API 1   │ │  API 2     │   │
│  └────────┘ └───────┘ └──────────┘ └────────────┘   │
└─────────────────────────────────────────────────────┘
```

## Components

### API Layer (`path/to/api/`)
Describe the router, middleware chain, and how requests are handled.

### Service Layer (`path/to/services/`)
Business logic. Describe how services are structured and their responsibilities.

### Repository Layer (`path/to/repo/`)
Data access. Describe the database driver, migration strategy, and patterns.

## Data Flow

Request → Router → Middleware → Handler → Service → Repository → Database
                                                  ↘ Cache

## External Integrations

| Service | Purpose | Required |
|---------|---------|----------|
| Database | Primary data store | Yes |
| Cache | Response caching, rate limiting | Yes |
| External API 1 | Description | Yes |
| External API 2 | Description | Optional |

## Infrastructure

- Hosting platform and configuration
- Database hosting
- Cache hosting
- File storage
```

### patterns.md

Coding conventions the team follows.

```markdown
---
type: patterns
updated: 2026-04-11
---

# Patterns

## Error Handling
- Services return `(result, error)` — never panic.
- Wrap errors with context: `fmt.Errorf("createUser: %w", err)`.
- API layer maps error types to HTTP codes via central middleware.

## Naming
- Interfaces: verb-er suffix (`UserReader`, `TokenValidator`).
- Constructors: `NewXxx(deps) *Xxx`.
- Files: snake_case matching the primary type they define.

## Testing
- Table-driven tests for pure functions.
- Integration tests hit real database (Docker via testcontainers).
- No mocks for database layer — test against real PostgreSQL.

## Git
- Conventional commits: `feat:`, `fix:`, `refactor:`, `docs:`.
- Squash merge to main. Feature branches: `feat/short-description`.
```

### bugs.md

Notable bugs with root cause analysis.

```markdown
---
type: bugs
updated: 2026-04-11
---

# Bugs

## Race condition in concurrent webhook delivery
**Date:** 2026-04-08
**Symptom:** Duplicate webhook deliveries under load.
**Root cause:** Webhook worker used shared slice without mutex. Two goroutines could read the same batch.
**Fix:** Added per-batch channel instead of shared slice. PR #142.
**Lesson:** Never share mutable state between goroutines without synchronization.
```

### features/*.md

One page per significant feature, tracking its full lifecycle. Created when a feature is first built. Updated when bugs are fixed, decisions change, or refactoring happens. This is the connective tissue — when someone asks "what's the story of X?", this page has the answer.

```markdown
---
type: feature
updated: 2026-05-20
---

# Webhook Delivery

## Overview
Async webhook delivery system that sends event payloads to subscriber endpoints with retry logic.

## Timeline

- **2026-01-15** — Initial implementation. Chose Redis queue over RabbitMQ for simplicity. See [[decisions.md#chose-redis-queue-for-webhooks]].
- **2026-03-08** — Added retry with exponential backoff (max 5 attempts, 30min cap).
- **2026-05-18** — Fixed race condition: shared slice in worker replaced with per-batch channel. See [[bugs.md#race-condition-in-webhook-delivery]].
- **2026-05-20** — Added dead letter queue for permanently failed deliveries.

## Current State
Working. Handles ~2k deliveries/min. Retry logic covers transient failures. Dead letters logged for manual review.

## Key Files
- `internal/webhook/dispatcher.go` — main delivery loop
- `internal/webhook/retry.go` — backoff logic
- `internal/webhook/worker.go` — per-batch workers
```

**When to create a feature page:** When a feature spans multiple sessions, involves architectural decisions, or is complex enough that a future developer would ask "what's the story here?" Not every small change needs one — only features that have a lifecycle.

**When a feature is removed from the codebase:**

**Never delete the feature page.** The WHY behind a removed feature is arguably more valuable than the WHY behind a current one — it explains the walked-away-from path. Instead:

1. Update `## Current State` at the top of the feature page:
   ```markdown
   ## Current State
   **Removed 2026-09-20** — see [[history.md#removed-webhook-delivery]] for context.
   Kept in brain as narrative record.
   ```
2. Keep the rest of the page (Overview, Timeline, Key Files) intact — it's historical record now.
3. Add a `history.md` entry explaining the removal with the WHY:
   ```markdown
   ## Removed webhook delivery feature
   **Date:** 2026-09-20
   Retired webhook delivery after migrating all subscribers to server-sent events. Webhook infra was load-bearing for 8 months but added operational complexity disproportionate to its use. See [[features/webhook-delivery.md]] for the lifecycle.
   ```
4. If clutter in `features/` becomes a problem after many removals, move removed features to `archive/features-<year>.md` following the same compaction rules — but this is a long-horizon cleanup, not a removal-time action.

The feature page going forward shows "Removed" status. Its Timeline records what happened; its `Key Files` list is now a pointer to what used to exist (still valuable for anyone spelunking git history).

### topics/*.md

A topic page synthesizes the story of a domain that crosses event boundaries — a subsystem (Redis, auth), a concept (caching strategy), a recurring concern (performance, security). Unlike `features/*.md` (one feature's full lifecycle), a topic touches multiple features, decisions, and bugs over time. Topic pages solve the "history of a topic is scattered across decisions/bugs/history/features" problem by providing a canonical narrative hub.

```markdown
---
type: topic
updated: 2026-04-16
---

# Redis

## Overview
Redis serves as the session cache and pub/sub backbone for cache invalidation across API instances. Adopted 2026-01.

## Timeline

- **2026-04-16** — Fixed OOM crash by capping maxmemory at 4GB and enabling LRU eviction. See [[bugs.md#redis-oom-crash-under-load]].
- **2026-03-22** — Fixed connection pool exhaustion during peak hours (pool size raised 20 → 100). See [[bugs.md#redis-connection-pool-exhaustion]].
- **2026-01-10** — Chose Redis over Memcached for pub/sub invalidation support. See [[decisions.md#chose-redis-over-memcached-for-session-caching]].

## Key Decisions
- [[decisions.md#chose-redis-over-memcached-for-session-caching]]
- [[decisions.md#redis-cluster-sharding-strategy]]

## Related
- Features: [[features/session-auth.md]], [[features/rate-limiting.md]]
- Topics: [[topics/caching.md]]

## Current Status
Active. Running Redis 7 in cluster mode with 3 shards. Memory-related incidents have recurred three times; architectural review is on the watchlist for 2026-Q3.
```

**Authoring rules:**
- Timeline entries are `**YYYY-MM-DD** — caption [[page.md#anchor]]`. The wikilink is authoritative; the caption is a hint for readers and may drift over time.
- **Caption quality: name the lever that moved.** Weak: `Fixed bug`. Strong: `Fixed Redis OOM by capping maxmemory at 4GB`. The goal is that a reader skimming the Timeline sees what actually changed, without having to follow the wikilink.
- **Creation is user-initiated only.** Create via `/brain topic <name>`. The LLM does NOT auto-create topic pages during `/brain update` or `/brain init` — this prevents weak, sticky topics.
- **Maintenance is automatic.** During `/brain update`, the LLM checks existing topics and appends Timeline wikilinks for any session events that match a topic's scope.
- **Not compacted.** Topic pages are the canonical narrative across archive boundaries. When an event page is compacted, the topic's Timeline wikilink may need to be repointed at the archive path — doctor flags this for manual repoint.
- **Kept small.** Topics rarely exceed 150 lines. If one grows too large, split it (e.g., `topics/redis.md` → `topics/redis-ops.md` + `topics/redis-data-model.md`).

**Topics vs Custom vs Features — decision tree:**

| Type | Shape | Example |
|------|-------|---------|
| `features/<name>.md` | One feature's full lifecycle | `features/webhook-delivery.md` |
| `topics/<name>.md`   | One domain's cross-cutting story | `topics/redis.md`, `topics/auth.md` |
| `custom/<name>.md`   | Team-defined reference docs | `custom/onboarding.md`, `custom/runbook-oncall.md` |

Ask these three questions in order:

```
 1. Does the page tell a chronological story with a Timeline section
    (events + dates + wikilinks)?
       │
       ├── YES → question 2
       │
       └── NO  → it's custom/*.md
                 (runbook, onboarding checklist, conventions doc,
                  incident postmortem template, etc.)

 2. Is it about ONE shipping unit of product (a discrete feature,
    integration, or user-facing capability)?
       │
       ├── YES → it's features/<name>.md
       │         (webhook-delivery, oauth-login, billing-portal)
       │
       └── NO  → question 3

 3. Is it about a DOMAIN that shows up across multiple features —
    a subsystem, a concept, or a recurring concern?
       │
       └── YES → it's topics/<name>.md
                 (redis, auth, caching, performance, security)
```

**In practice:**
- `features/oauth-login.md` — the lifecycle of *the OAuth login feature*: when shipped, what changed, which bugs hit it
- `topics/auth.md` — *how authentication works in this project* as a whole — session storage, token rotation, OAuth providers, MFA — cross-references features/oauth-login.md, features/mfa.md, etc.
- `custom/oncall-runbook.md` — a reference the oncall engineer reads during an incident; no chronology

**When a page grows past its type:** a feature page that turns into a cross-cutting narrative (happens when one feature absorbs multiple subsystems) should be renamed and moved to `topics/`. The Timeline wikilinks in other pages pointing at the old path must be updated — doctor flags this.

## Linking with [[wikilinks]]

Brain pages reference each other using `[[wikilinks]]`. This connects entries across pages so the LLM can trace a feature's full story.

### Syntax

```
[[page.md]]                              → link to a page
[[page.md#section-anchor]]               → link to a specific section
[[features/webhook-delivery.md]]         → link to a feature page
```

### Anchor Slug Algorithm

Section headers become wikilink anchors via a deterministic slug algorithm. Every LLM that reads and writes wikilinks must apply this algorithm identically — otherwise links drift silently across sessions/tools.

**Algorithm (GitHub-compatible):**

1. Take the header text after the `##`/`###` marker.
2. Strip leading/trailing whitespace.
3. Lowercase everything.
4. Remove all characters except: lowercase letters `a-z`, digits `0-9`, Unicode letters (ç, ş, ñ, etc.), whitespace, and hyphens `-`. Everything else — punctuation, backticks, em-dashes, slashes, colons, quotes, parentheses — is DELETED (not replaced with hyphens).
5. Replace each run of whitespace with a single hyphen.
6. Collapse consecutive hyphens to one.
7. Strip leading and trailing hyphens.

**Reference implementation (Python):**

```python
import re, unicodedata

def slugify(header: str) -> str:
    s = header.strip().lower()
    # keep letters (incl. unicode), digits, whitespace, hyphens; drop the rest
    s = ''.join(c for c in s
                if c.isalnum() or c.isspace() or c == '-'
                or unicodedata.category(c).startswith('L'))
    s = re.sub(r'\s+', '-', s)      # whitespace → hyphen
    s = re.sub(r'-+', '-', s)       # collapse runs
    return s.strip('-')
```

**Worked examples:**

| Header | Slug |
|--------|------|
| `## Chose Redis Queue for Webhooks` | `chose-redis-queue-for-webhooks` |
| `## Race Condition in Webhook Delivery` | `race-condition-in-webhook-delivery` |
| `## Redis — the \`SET\` command` | `redis-the-set-command` |
| `## Why do we use JWTs?` | `why-do-we-use-jwts` |
| `## Auth: tokens, cookies, and you` | `auth-tokens-cookies-and-you` |
| `## Upgraded Postgres 14 → 16` | `upgraded-postgres-14-16` |
| `## HTTP/2 rollout` | `http2-rollout` |
| `## v2.0 release (2026-04-01)` | `v20-release-2026-04-01` |
| `## Rate-limit: 100 req/sec` | `rate-limit-100-reqsec` |
| `## Öğrenci kaydı akışı` | `öğrenci-kaydı-akışı` |

The algorithm is stable: given the same header text, every tool and LLM produces the same slug. When in doubt about an edge case, run the Python reference above mentally — it's the source of truth.

### When to Link

| Situation | Link from → to |
|-----------|---------------|
| Bug fix references original feature | `bugs.md` → `[[features/X.md]]` |
| Decision relates to a feature | `decisions.md` → `[[features/X.md]]` |
| History entry about a feature | `history.md` → `[[features/X.md]]` |
| Feature references its decisions | `features/X.md` → `[[decisions.md#anchor]]` |
| Feature references its bugs | `features/X.md` → `[[bugs.md#anchor]]` |
| Bug caused by an architectural decision | `bugs.md` → `[[decisions.md#anchor]]` |
| Any entry to its domain topic | `bugs.md` / `decisions.md` / `features/X.md` → `[[topics/Y.md]]` |
| Topic's Timeline back to its events | `topics/Y.md` → `[[decisions.md#anchor]]`, `[[bugs.md#anchor]]`, `[[features/X.md]]` |
| Feature references a broader topic | `features/X.md` `## Related` → `[[topics/Y.md]]` |

### How the LLM Uses Links

When a developer asks about a topic, the LLM:
1. Greps all `.brain/` files for the keyword
2. Finds the most relevant page (often a feature page)
3. Follows `[[wikilinks]]` to gather related entries from other pages
4. Synthesizes the full story from all connected entries

This is what makes `.brain/` a wiki, not a log.

## Date Format

Every entry in `history.md`, `decisions.md`, `bugs.md`, and `features/*.md` timelines MUST have a `**Date:** YYYY-MM-DD` line immediately after the `## ` header. Full date required — never month-only (`2026-03`), never relative. This enables chronological sorting in the dashboard.

**Exact format required:** `YYYY-MM-DD` where YYYY is 4 digits, MM is 01-12 (with leading zero), DD is 01-31 (with leading zero). Must match regex `^\d{4}-\d{2}-\d{2}$`.

**Correct:**
```markdown
## Chose Redis over Memcached
**Date:** 2026-04-10
Content here...
```

**Wrong examples:**
- `2026-4-10` — missing leading zero on month
- `2026-04-5` — missing leading zero on day
- `04/10/2026` — wrong separator
- `April 10, 2026` — text instead of digits
- `10-04-2026` — wrong order (DD-MM-YYYY)

**Wrong placement:**
```markdown
## 2026-04-10 — Chose Redis over Memcached
Content here...
```

The date goes in a `**Date:**` field, not in the header. Headers are for titles only.

Also: frontmatter `updated:` field uses the same YYYY-MM-DD format.

## What NOT to Put in .brain/

- Code snippets longer than 5 lines (link to the file instead)
- Secrets, API keys, credentials
- Personal opinions or complaints
- Speculative future plans (only record what IS, not what MIGHT BE)
- Duplicate information already in README.md (reference it instead)

## Compaction

When a page exceeds **30 entries** or **150 lines**, compact it:

1. Create `archive/` directory: `mkdir -p .brain/archive`
2. Move entries older than 3 months to `.brain/archive/<page>-<year>.md`
3. Add a summary line at the bottom of the active page:

```markdown
> Older entries archived in [archive/history-2025.md](archive/history-2025.md)
```

Archive files use the same frontmatter with `type: archive`. They are still in git, still searchable, but not loaded into LLM context at session start.

**When an archive file itself exceeds 150 lines**, sub-split it by half-year:
- `archive/history-2024.md` → split into `archive/history-2024-h1.md` (Jan–Jun) and `archive/history-2024-h2.md` (Jul–Dec), sorting entries into the correct half by `**Date:**`.
- Update the active page's pointer line to reference both halves:
  ```markdown
  > Older entries archived in [archive/history-2024-h1.md](archive/history-2024-h1.md), [archive/history-2024-h2.md](archive/history-2024-h2.md), [archive/history-2025.md](archive/history-2025.md).
  ```
- If a half-year bucket itself exceeds 150 lines (rare, only for extremely active pages), split further by quarter: `-q1.md`, `-q2.md`, etc.
- Any topic Timeline wikilinks pointing at the pre-split archive path must be re-targeted — doctor will flag them.

**Topic pages are NOT compacted.** `topics/*.md` pages serve as the canonical narrative across archive boundaries. When an event is moved from `decisions.md` to `archive/decisions-2025.md`, any topic Timeline wikilink that pointed at the original path (e.g., `[[decisions.md#chose-redis]]`) will break — the doctor flags these with a specific recovery hint ("slug matches header in `archive/...` — repoint the wikilink"). This is the expected workflow: compact the event page, then repoint the topic's Timeline entry.

## Merge Conflict Guidance

Brain pages are designed to be merge-friendly, but newest-first ordering in
`history.md`, `decisions.md`, `bugs.md` means parallel entries land at the same
line position and git flags a conflict. When you hit one, classify the conflict
first — the resolution depends on which case it is.

### Case 1 — Both sides added new entries (the common case)

Both branches inserted a new `## ` section just under the page's `#` heading,
OR both branches appended Timeline bullets under a `## Timeline` section
(applies to `features/*.md` and `topics/*.md` Timelines as well as
`history.md`, `decisions.md`, `bugs.md`).
This is a false-alarm conflict: both entries are correct, non-overlapping work.

Resolve mechanically:
1. Keep both entries — do not drop either side.
2. Sort all entries under the `#` heading (or under `## Timeline`) by
   `**Date:** YYYY-MM-DD` descending (newest first).
3. Set frontmatter `updated:` to the max date across all entries in the file.
4. Delete the `<<<<<<<`, `=======`, `>>>>>>>` markers.

No human judgment is needed for this case.

### Case 2 — Both sides edited the same existing entry (rare but real)

Two branches modified the body, status, or metadata of the same `## ` section
(e.g., both changed `**Status:**`, both added a "Follow-up" note, one fixed a
typo while the other added a wikilink). This is a genuine coordination issue.

Do not mechanically union — you may produce a nonsense entry with duplicate
`**Date:**` or `**Status:**` lines.

Resolve as follows:
1. Read both versions carefully.
2. If both updates are valid, combine them into a single coherent entry.
3. If they contradict (e.g., `Status: Fixed` vs `Status: WontFix`), pick the
   correct one based on the actual project state — read the code, check git
   history, or ask the developer.
4. Note the reconciliation briefly in the entry body if the conflict revealed
   a disagreement worth remembering.

### Case 3 — Conflicts in `index.md`, `architecture.md`, `patterns.md`

These are prose pages, not append-only lists. Read both versions, pick the
more current one, or combine if both changes are valid. When in doubt, keep
both versions inline and let the next session reconcile.

### Case 4 — Both branches added the same new topic or feature page (add+add)

Two branches independently ran `/brain topic <name>` (or created `features/<name>.md`) with the same slug. Git produces an **add+add file conflict** — the file exists on both sides with different content.

Unlike Case 1 (both sides append within an existing file), this is the file itself being newly created on both sides. Git can't 3-way merge it because there's no common ancestor version of the file.

Resolve:
1. **Overview section:** read both sides. Combine into one coherent paragraph, preserving both perspectives where they don't contradict. If they contradict (e.g., different Overview framings), pick the one that better matches current project state.
2. **Timeline section:** concatenate bullets from both sides; sort by `**Date:**` descending; de-duplicate bullets that point at the same wikilink.
3. **Key Decisions / Related / Current Status:** merge without duplication. If one side has richer content, prefer it.
4. Set frontmatter `updated:` to the max date across all Timeline bullets in the merged file.
5. Resolve the conflict in git as a single merged file.

This applies identically to topic pages and feature pages.

## Platform Integration

The repo's `CLAUDE.md`, `.cursor/rules`, and `AGENTS.md` point LLM tools to this file:

**CLAUDE.md** (Claude Code):
```
# Project Brain
This repo has .brain/ project memory. Read .brain/index.md for project context.
When updating brain pages, read .brain/SCHEMA.md for format rules and update instructions.
```

**.cursor/rules** (Cursor):
```
This repo has .brain/ project memory. Read .brain/index.md for project context.
When updating brain pages, read .brain/SCHEMA.md for format rules and update instructions.
```

**AGENTS.md** (Codex):
```
This repo has .brain/ project memory. Read .brain/index.md for project context.
When updating brain pages, read .brain/SCHEMA.md for format rules and update instructions.
```

## Custom Pages

Teams can add domain-specific pages under `.brain/custom/`:

```
.brain/custom/
├── api-versioning.md    # API versioning strategy
├── deployment.md        # Deploy process and rollback procedures
├── onboarding.md        # New developer setup guide
└── incidents.md         # Post-mortem summaries
```

Custom pages use the same frontmatter format with `type: custom`.
