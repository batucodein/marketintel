DROP INDEX IF EXISTS ix_campaigns_market_id;
ALTER TABLE campaigns DROP COLUMN IF EXISTS sender_profile_id;
ALTER TABLE campaigns DROP COLUMN IF EXISTS market_id;
