-- Sender-profile-aware scoring foundations.
-- Adds positioning fields the AI scorer reads alongside business shipment +
-- website data so leads are ranked against THIS user's specific ICP, not
-- just the market's product. All optional / free-text — the AI interprets.

ALTER TABLE sender_profiles
    ADD COLUMN IF NOT EXISTS target_industries     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_countries      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS avoid_countries       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS min_deal_size_usd     BIGINT,
    ADD COLUMN IF NOT EXISTS typical_deal_size_usd BIGINT,
    ADD COLUMN IF NOT EXISTS deal_breakers         TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS competitive_moats     TEXT NOT NULL DEFAULT '';
