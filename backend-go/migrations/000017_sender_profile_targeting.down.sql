ALTER TABLE sender_profiles
    DROP COLUMN IF EXISTS competitive_moats,
    DROP COLUMN IF EXISTS deal_breakers,
    DROP COLUMN IF EXISTS typical_deal_size_usd,
    DROP COLUMN IF EXISTS min_deal_size_usd,
    DROP COLUMN IF EXISTS avoid_countries,
    DROP COLUMN IF EXISTS target_countries,
    DROP COLUMN IF EXISTS target_industries;
