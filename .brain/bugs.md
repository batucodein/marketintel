---
type: bugs
updated: 2026-04-20
---

# Bugs

## AI draft save failed — ai_prompt_version varchar(16) too tight

**Date:** 2026-04-20
**Symptom:** Clicking "Email" on a lead AI-drafted a message but persisting it returned `ERROR: value too long for type character varying(16) (SQLSTATE 22001)`.
**Root cause:** `messages.ai_prompt_version` is `VARCHAR(16)` (migration 9), but the service writes `"outreach_draft_v1"` / `"outreach_reply_v1"` — both 17 characters.
**Fix:** Migration 10 bumped the column to `VARCHAR(64)`.
**Lesson:** Second instance of the varchar-too-tight pattern after the country-code `UNKNOWN` overflow (bugs below). Whenever code writes a constant string into a varchar column, grep for the column width before shipping. Better: default to `VARCHAR(64)` or `TEXT` for identifier-ish fields that don't benefit from a tight bound.

## Inline contact edit failed with "Load failed" — CORS missing PATCH

**Date:** 2026-04-20
**Symptom:** After adding the manual contact-info edit feature on the lead drawer, clicking Save showed "Load failed" in the UI. `PATCH /markets/{id}/leads/{businessID}` never reached the backend.
**Root cause:** The global CORS middleware in `internal/server/server.go` had `AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}` — no `PATCH`. The browser's preflight OPTIONS returned 200 but without `Access-Control-Allow-Methods` containing PATCH, so the browser aborted the actual PATCH request. The frontend fetch error surfaced as a generic "Load failed" because it happened before any response came back.
**Fix:** Added `"PATCH"` to the `AllowedMethods` list.
**Lesson:** Any time a new HTTP verb is added to a route in this repo, update the global CORS `AllowedMethods` list in `server.go`. Frontend errors that read "Load failed" or "Failed to fetch" with no HTTP status are almost always CORS preflight failures — check the Network tab for an OPTIONS request without the expected `Allow-*` headers before debugging anything else.

## UNKNOWN country fallback overflowed varchar(3)

**Date:** 2026-04-18
**Symptom:** Excel uploads crashed with `SQLSTATE 22001: value too long for type character varying(3)`. Surfaced as an error toast on the upload page: `"upload failed: create market for UNKNOWN: create market: ERROR: value too long for type character varying(3)"`.
**Root cause:** `SplitByDestination` bucketed records with unrecognized ConsigneeCountry into `"UNKNOWN"` (7 chars). The `markets.country_code` column is `varchar(3)`. Inserting `"UNKNOWN"` exceeded the column width.
**Fix:** Changed the fallback to `"ZZ"` (2 chars, officially an ISO-2 unassigned code). `ProcessExcel` now skips groups with `DestinationCountry == "ZZ"` and emits a warning in the response instead of crashing. Also broadened column-name matching in the parser: tries `"Consignee Country(EN)"`, `"Consignee Country"`, `"Destination Country"`, `"Country"` — Tendata and similar vendors use inconsistent headers.
**Lesson:** Always verify that fallback values for DB-bound strings fit the schema. A 3-char column is a hard constraint; fallbacks must respect it. Also added to [[patterns.md#iso-2-fallback-code-zz-for-unknown]].

## Async enrichment silently killed by Cloud Run CPU throttling

**Date:** 2026-04-18
**Symptom:** Market cards never received their `derived_product_name` — AI summarization didn't appear to run. Logs showed the HTTP handler returning successfully but the goroutines produced no output.
**Root cause:** Default Cloud Run deallocates CPU from an instance when all HTTP requests finish. Our goroutines spawned by `ProcessExcel` got CPU-starved and died before doing any work.
**Fix:** `gcloud run services update marketintel --no-cpu-throttling`. This makes Cloud Run bill per instance-second (not per request) but keeps the CPU hot while the instance is alive. Also set `--timeout=1800` and kept `--min-instances=0` so the instance still scales to zero when truly idle. Frontend polling every 3s keeps the instance warm while enrichment is running.
**Lesson:** Serverless platforms have subtle execution-lifetime rules. Background goroutines only work on Cloud Run with `--no-cpu-throttling`. Document this in [[architecture.md]] so future changes don't silently regress it. Tracked as a v2 concern to move to Cloud Tasks if uploads outgrow the 30-minute request timeout.

## Embedded `.git` inside frontend/ almost became a broken submodule

**Date:** 2026-04-18
**Symptom:** First `git add -A` on the newly-created marketintel repo emitted `warning: adding embedded git repository: frontend`. Had the push gone ahead, the frontend would have been registered as an unresolvable gitlink.
**Root cause:** The frontend directory was initialized with its own `.git` during Next.js scaffolding and never cleaned up.
**Fix:** `rm -rf frontend/.git` then `git rm --cached -f frontend` and re-add.
**Lesson:** Before the first push of a monorepo, check for nested `.git` directories. Added to [[patterns.md#pre-push-safety-verify-secrets-and-nested-repos-before-git-add-a]].

## Market cards showed old destination-country-first name with a date in parens

**Date:** 2026-04-18
**Symptom:** Existing markets in the UI showed titles like `CA — HS 6802.91 (2026-04-18)` even after the AI product-name feature was deployed.
**Root cause:** Two-part issue. (1) Older markets were created before the naming format was cleaned up, so their `name` column held the old string. (2) Their `derived_product_name` was NULL because the AI enrichment goroutines had died (see [[bugs.md#async-enrichment-silently-killed-by-cloud-run-cpu-throttling]]).
**Fix:** New market naming is `HS 6802.91 — CA` (short, no date — date shows separately on the card). Old markets stay as-is; users can delete and re-upload, or a future backfill endpoint can re-derive names. Real fix for new uploads is the CPU-throttling fix above.
**Lesson:** When UI regressions occur, separate "cosmetic" from "data" causes. The visible issue was the title; the hidden issue was the missing AI data.
