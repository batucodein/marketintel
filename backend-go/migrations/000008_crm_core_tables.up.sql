-- Outreach & CRM P1 — core tables
-- Contacts, user sending channels, and sender profiles.

-- A user's CRM relationship with a business.
-- One contact per (user, business) — surviving across markets.
CREATE TABLE contacts (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    business_id         UUID NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
    primary_email       VARCHAR(255),
    primary_phone       VARCHAR(50),
    display_name        TEXT NOT NULL,
    pipeline_stage      VARCHAR(32) NOT NULL DEFAULT 'lead',
    default_automation  VARCHAR(16) NOT NULL DEFAULT 'manual',
    default_sequence_id UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_contact_user_business UNIQUE (user_id, business_id)
);
CREATE INDEX ix_contacts_user ON contacts (user_id);
CREATE INDEX ix_contacts_pipeline ON contacts (user_id, pipeline_stage);

-- Connected sending channels per user (Gmail OAuth, later Outlook, SMTP, WhatsApp).
CREATE TABLE user_channels (
    id                              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id                         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type                            VARCHAR(32) NOT NULL,
    display_label                   TEXT NOT NULL,
    from_email                      VARCHAR(255) NOT NULL,
    oauth_access_token_encrypted    TEXT,
    oauth_refresh_token_encrypted   TEXT,
    oauth_expires_at                TIMESTAMPTZ,
    oauth_scope                     TEXT,
    config_encrypted                JSONB,
    enabled                         BOOLEAN NOT NULL DEFAULT true,
    is_default                      BOOLEAN NOT NULL DEFAULT false,
    last_poll_at                    TIMESTAMPTZ,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_user_channels_user ON user_channels (user_id);
CREATE INDEX ix_user_channels_enabled ON user_channels (user_id, enabled);
CREATE UNIQUE INDEX ix_user_channels_default ON user_channels (user_id) WHERE is_default IS TRUE;

-- Sender profile — user's positioning for AI-generated emails.
CREATE TABLE sender_profiles (
    user_id                   UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    company_name              TEXT NOT NULL DEFAULT '',
    product_description       TEXT NOT NULL DEFAULT '',
    value_prop                TEXT NOT NULL DEFAULT '',
    target_buyer_description  TEXT NOT NULL DEFAULT '',
    tone                      TEXT NOT NULL DEFAULT 'formal',
    signature                 TEXT NOT NULL DEFAULT '',
    default_channel_id        UUID REFERENCES user_channels(id) ON DELETE SET NULL,
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);
