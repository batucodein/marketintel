-- Add Google Places enrichment columns to businesses
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS rating NUMERIC(2,1);
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS rating_count INTEGER;
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS google_types JSONB;
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS opening_hours JSONB;

-- Ensure lead_scores has a unique constraint on (business_id, market_id, user_id)
-- so upserts work correctly and we don't accumulate duplicate scores.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'uq_lead_scores_business_market_user'
    ) THEN
        ALTER TABLE lead_scores
            ADD CONSTRAINT uq_lead_scores_business_market_user
            UNIQUE (business_id, market_id, user_id);
    END IF;
END
$$;
