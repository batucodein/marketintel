-- Dynamic ingestion + existence-aware scoring foundations.
-- Adds:
--   1. businesses.input_field_presence — which canonical fields the source
--      Excel actually contained for THIS row. Powers per-dimension scoring
--      caps so a row with no email can never score high on accessibility.
--   2. lead_scores.dimension_completeness — per-dimension data quality
--      reported by the AI and validated/clamped server-side.
--   3. search_column_mappings — the resolved header → canonical mapping
--      the user confirmed for this upload, plus AI confidence + overrides.

ALTER TABLE businesses
    ADD COLUMN IF NOT EXISTS input_field_presence JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE lead_scores
    ADD COLUMN IF NOT EXISTS dimension_completeness JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS search_column_mappings (
    search_id      UUID PRIMARY KEY REFERENCES searches(id) ON DELETE CASCADE,
    mapping        JSONB NOT NULL,                          -- {header: canonical_field}
    ai_confidence  JSONB NOT NULL DEFAULT '{}'::jsonb,      -- {header: 0.0-1.0}
    user_overrides JSONB NOT NULL DEFAULT '{}'::jsonb,      -- {header: canonical_field}
    unmapped       JSONB NOT NULL DEFAULT '[]'::jsonb,      -- [header, ...]
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
