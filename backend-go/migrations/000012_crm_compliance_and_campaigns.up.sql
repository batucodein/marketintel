-- P2 — Campaigns + compliance (CAN-SPAM/GDPR baseline).
-- Adds: campaigns, campaign_contacts, unsubscribes; extends sender_profiles
-- with physical_address; extends contacts with unsubscribed_at; activates
-- the campaign_id FK on conversations and messages (placeholders since 0009).

-- Per-channel HMAC secret for signing one-click unsubscribe URLs.
ALTER TABLE user_channels
    ADD COLUMN IF NOT EXISTS unsubscribe_secret BYTEA;

-- Backfill secrets for any existing channels (32 random bytes).
UPDATE user_channels SET unsubscribe_secret = gen_random_bytes(32)
WHERE unsubscribe_secret IS NULL;

ALTER TABLE user_channels
    ALTER COLUMN unsubscribe_secret SET NOT NULL;

-- CAN-SPAM requires a physical mailing address in commercial email.
-- Validated at campaign-launch time, not at profile-save time, so existing
-- P1 single-send users keep working.
ALTER TABLE sender_profiles
    ADD COLUMN IF NOT EXISTS physical_address TEXT NOT NULL DEFAULT '';

-- Suppression flag on the contact. Set when user clicks unsubscribe URL,
-- replies with an opt-out keyword, or hits the List-Unsubscribe header.
ALTER TABLE contacts
    ADD COLUMN IF NOT EXISTS unsubscribed_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS unsubscribe_reason TEXT;

CREATE INDEX IF NOT EXISTS ix_contacts_unsubscribed ON contacts (user_id)
    WHERE unsubscribed_at IS NOT NULL;

-- A named batch of outreach with a shared goal/positioning.
CREATE TABLE campaigns (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id               UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id            UUID NOT NULL REFERENCES user_channels(id) ON DELETE RESTRICT,
    name                  TEXT NOT NULL,
    goal                  TEXT NOT NULL DEFAULT '',
    status                VARCHAR(16) NOT NULL DEFAULT 'draft',
    -- 'draft' | 'ready' | 'active' | 'paused' | 'completed' | 'stopped'
    positioning_override  JSONB,                 -- null = use sender_profile
    sequence_id           UUID,                  -- FK added in migration 13
    send_pace_per_day     INTEGER NOT NULL DEFAULT 50,
    attach_catalog        BOOLEAN NOT NULL DEFAULT false,
    start_at              TIMESTAMPTZ,
    started_at            TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_campaigns_user_status ON campaigns (user_id, status, created_at DESC);

-- Membership: which contacts are in which campaigns + per-row state.
CREATE TABLE campaign_contacts (
    campaign_id        UUID NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    contact_id         UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    market_id          UUID,                     -- nullable; sourced market for context
    status             VARCHAR(16) NOT NULL DEFAULT 'pending',
    -- 'pending' | 'drafted' | 'approved' | 'sent' | 'replied' | 'cold' | 'skipped' | 'failed'
    draft_message_id   UUID REFERENCES messages(id) ON DELETE SET NULL,
    conversation_id    UUID REFERENCES conversations(id) ON DELETE SET NULL,
    scheduled_send_at  TIMESTAMPTZ,
    sent_at            TIMESTAMPTZ,
    replied_at         TIMESTAMPTZ,
    skip_reason        TEXT,
    added_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, contact_id)
);
CREATE INDEX ix_campaign_contacts_status ON campaign_contacts (campaign_id, status);
-- Drafter picks up pending rows in arrival order.
CREATE INDEX ix_campaign_contacts_pending ON campaign_contacts (added_at)
    WHERE status = 'pending';
-- Scheduler picks up due approved rows.
CREATE INDEX ix_campaign_contacts_due ON campaign_contacts (scheduled_send_at)
    WHERE status = 'approved';

-- Activate the FKs from conversations.campaign_id and messages.campaign_id.
-- These columns already exist as nullable placeholders since 000009.
ALTER TABLE conversations
    ADD CONSTRAINT fk_conversations_campaign
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE SET NULL;

ALTER TABLE messages
    ADD CONSTRAINT fk_messages_campaign
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE SET NULL;

CREATE INDEX ix_messages_campaign ON messages (campaign_id) WHERE campaign_id IS NOT NULL;
