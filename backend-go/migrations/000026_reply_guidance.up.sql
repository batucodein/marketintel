-- Reply drafting guidance.
-- Two sources feed the reply-draft prompt's "standing guidance" block, both
-- keyed off the inbound reply's intent tags:
--   1. campaign_tag_guidance — the per-group playbook the user authors per tag.
--   2. brand_reply_lessons   — per-brand lessons the user explicitly chose to
--      remember from a draft refinement, matched by tags + sentiment so a lesson
--      only applies to the same kind of buyer reply.

CREATE TABLE IF NOT EXISTS campaign_tag_guidance (
    campaign_id UUID        NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    tag         VARCHAR(32) NOT NULL,
    instruction TEXT        NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (campaign_id, tag)
);

CREATE TABLE IF NOT EXISTS brand_reply_lessons (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID        NOT NULL,
    sender_profile_id UUID        NOT NULL REFERENCES sender_profiles(id) ON DELETE CASCADE,
    instruction       TEXT        NOT NULL,
    match_tags        TEXT[]      NOT NULL DEFAULT '{}',
    match_sentiment   VARCHAR(16) NOT NULL,   -- positive | neutral | negative
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_brand_reply_lessons_brand
    ON brand_reply_lessons (sender_profile_id, match_sentiment);
