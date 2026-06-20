-- Recreate the directives table (rollback). The feature code is gone, so this
-- only restores the empty schema.
CREATE TABLE IF NOT EXISTS campaign_draft_directives (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID        NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    instruction TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_campaign_draft_directives
    ON campaign_draft_directives (campaign_id, created_at);
