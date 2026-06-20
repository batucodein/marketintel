-- An Email Group is a campaign scoped to a market, sending as that market's
-- brand. Both nullable so existing campaigns keep working (workers fall back
-- to the user's Default brand when sender_profile_id is null).
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS market_id UUID
    REFERENCES markets(id) ON DELETE SET NULL;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS sender_profile_id UUID
    REFERENCES sender_profiles(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS ix_campaigns_market_id ON campaigns (market_id);
