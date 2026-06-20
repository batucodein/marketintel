-- Standing draft directives: per-group (campaign) instructions the user gives
-- the AI about how to write ALL drafts in the group ("never mention X",
-- "always lead with lead time"). Applied to every existing draft when issued
-- (bulk rewrite) and injected into the prompt for every future draft.
CREATE TABLE IF NOT EXISTS campaign_draft_directives (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID        NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    instruction TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_campaign_draft_directives
    ON campaign_draft_directives (campaign_id, created_at);
