-- Reverse Google Places enrichment changes
ALTER TABLE businesses DROP COLUMN IF EXISTS rating;
ALTER TABLE businesses DROP COLUMN IF EXISTS rating_count;
ALTER TABLE businesses DROP COLUMN IF EXISTS google_types;
ALTER TABLE businesses DROP COLUMN IF EXISTS opening_hours;

ALTER TABLE lead_scores DROP CONSTRAINT IF EXISTS uq_lead_scores_business_market_user;
