---
type: decisions
updated: 2026-04-11
---

# Decisions

## Rewrote backend from Python/FastAPI to Go
**Date:** 2026-03-01
**Context:** Original prototype was Python/FastAPI. Needed better performance and type safety for the multi-stage pipeline with concurrent API calls.
**Decision:** Full rewrite in Go with Chi router. Leverages goroutines and errgroup for concurrent external API calls in the pipeline.
**Status:** Active

## Multi-provider AI routing instead of single provider
**Date:** 2026-03-10
**Context:** Different AI tasks have different cost/quality tradeoffs. HS detection needs precision (low temperature), scoring needs nuance (higher temperature), simple queries can use cheaper models.
**Decision:** Built a provider router that maps task types to specific models and temperatures. Default is Azure OpenAI (gpt-5.4-mini for most tasks, gpt-5.4-nano for simple ones). Anthropic and Gemini available as alternatives.
**Status:** Active

## TOPSIS algorithm for market ranking
**Date:** 2026-03-12
**Context:** Needed to rank countries by multiple criteria (import value, GDP, population, accessibility, reliability). Simple weighted sum doesn't handle normalization well across different scales.
**Decision:** TOPSIS (Technique for Order of Preference by Similarity to Ideal Solution) — normalizes all criteria and finds the option closest to the ideal and farthest from the worst.
**Status:** Active

## Redis caching for AI responses
**Date:** 2026-03-15
**Context:** Same product/market combinations produce identical AI calls. Repeated calls waste money and add latency.
**Decision:** Cache AI completions in Redis, keyed by (provider, model, system prompt, user prompt, temperature). All cache hits/misses logged in ai_request_log.
**Status:** Active

## DB and Redis in Docker, Go runs locally
**Date:** 2026-03-01
**Context:** Faster development iteration. Docker for stateful services, local execution for the API binary.
**Decision:** docker-compose runs PostgreSQL and Redis. Backend runs via `go run ./cmd/server` or `make run`. Frontend runs via `next dev`.
**Status:** Active
