-- Outreach P1 — conversations and messages.
-- Channel-agnostic. Today: Gmail. Later: WhatsApp, LinkedIn, etc.

CREATE TABLE conversations (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id          UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    channel_id          UUID NOT NULL REFERENCES user_channels(id) ON DELETE CASCADE,
    campaign_id         UUID,  -- FK added in migration 10 when campaigns table exists
    channel_type        VARCHAR(32) NOT NULL,
    subject             TEXT,
    external_thread_id  TEXT,
    automation          VARCHAR(16) NOT NULL DEFAULT 'manual',
    status              VARCHAR(16) NOT NULL DEFAULT 'active',
    last_message_at     TIMESTAMPTZ,
    last_direction      VARCHAR(8),
    unread              BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_conversations_user_unread ON conversations (user_id, unread, last_message_at DESC);
CREATE INDEX ix_conversations_contact ON conversations (contact_id);
CREATE INDEX ix_conversations_channel ON conversations (channel_id);
CREATE UNIQUE INDEX ix_conversations_thread ON conversations (channel_id, external_thread_id) WHERE external_thread_id IS NOT NULL;

CREATE TABLE messages (
    id                       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    conversation_id          UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    direction                VARCHAR(8) NOT NULL,
    channel_type             VARCHAR(32) NOT NULL,
    external_id              TEXT,
    in_reply_to_external_id  TEXT,
    subject                  TEXT,
    body_text                TEXT,
    body_html                TEXT,
    ai_generated             BOOLEAN NOT NULL DEFAULT false,
    ai_model                 VARCHAR(64),
    ai_prompt_version        VARCHAR(16),
    status                   VARCHAR(32) NOT NULL DEFAULT 'sent',
    campaign_id              UUID,  -- FK added in migration 10
    sequence_step_id         UUID,  -- FK added in migration 10
    sent_at                  TIMESTAMPTZ,
    received_at              TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_messages_conversation ON messages (conversation_id, created_at);
CREATE INDEX ix_messages_external ON messages (external_id) WHERE external_id IS NOT NULL;
