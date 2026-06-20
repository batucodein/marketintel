-- Per-market brand: each market maps to exactly one sender profile.
-- Groups created from a market inherit this brand. ON DELETE SET NULL so
-- deleting a brand doesn't block the market — the create-group guard catches
-- a NULL brand instead.
ALTER TABLE markets ADD COLUMN IF NOT EXISTS sender_profile_id UUID
    REFERENCES sender_profiles(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS ix_markets_sender_profile_id ON markets (sender_profile_id);
