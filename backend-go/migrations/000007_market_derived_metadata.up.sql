-- Add deterministic metadata derived from Excel uploads
ALTER TABLE markets
    ADD COLUMN IF NOT EXISTS derived_product_name TEXT,
    ADD COLUMN IF NOT EXISTS dominant_hs_code     TEXT,
    ADD COLUMN IF NOT EXISTS all_hs_codes         TEXT[],
    ADD COLUMN IF NOT EXISTS origin_country       TEXT,
    ADD COLUMN IF NOT EXISTS origin_countries     TEXT[],
    ADD COLUMN IF NOT EXISTS origin_share         NUMERIC(5, 4),
    ADD COLUMN IF NOT EXISTS shipment_from_date   DATE,
    ADD COLUMN IF NOT EXISTS shipment_to_date     DATE,
    ADD COLUMN IF NOT EXISTS importer_count       INTEGER,
    ADD COLUMN IF NOT EXISTS shipment_count       INTEGER,
    ADD COLUMN IF NOT EXISTS source_file_name     TEXT,
    ADD COLUMN IF NOT EXISTS uploaded_at          TIMESTAMPTZ;

-- Legacy searches from the 4-stage flow would break on the new UI.
-- Set their status to 'failed' so they hide from the UI while preserving ai_request_log foreign keys.
UPDATE searches
SET status = 'failed', updated_at = now()
WHERE status IN ('pending', 'hs_detected', 'markets_ranked', 'explored');

-- Drop market analyses (feature removed)
DELETE FROM market_analyses;
