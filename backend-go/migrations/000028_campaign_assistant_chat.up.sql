-- Conversational draft-assistant chat: per-group (campaign) message history.
-- The assistant answers questions about the group's drafts and PROPOSES standing
-- directives the user confirms; confirmed proposals run the existing
-- ApplyDirective executor. Persisted so the thread survives reload (like ChatGPT).
CREATE TABLE IF NOT EXISTS campaign_assistant_messages (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id UUID        NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    role        VARCHAR(12) NOT NULL,               -- 'user' | 'assistant'
    content     TEXT        NOT NULL,
    -- Set on an assistant message that proposes a write action (pending confirm):
    -- {type: edit_drafts|add_playbook|remove_playbook, scope?, instruction?, tag?}.
    proposed_action JSONB,
    -- Proposal lifecycle: 'sent' (plain message) | 'proposed' | 'applied' | 'dismissed'.
    status      VARCHAR(12) NOT NULL DEFAULT 'sent',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_campaign_assistant_messages
    ON campaign_assistant_messages (campaign_id, created_at);
