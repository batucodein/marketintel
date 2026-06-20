-- Sentiment scoring + intent tags.
-- Phase 2: a numeric sentiment score (-1.0..1.0) per inbound reply, plus a
-- derived 5-level label. The score is the source of truth; the label is
-- denormalized so faceted filters can index it, and is re-thresholdable with a
-- single UPDATE (no AI re-classification). Legacy VARCHAR(16) sentiment columns
-- from migration 24 stay for back-compat during rollout.
ALTER TABLE messages      ADD COLUMN IF NOT EXISTS sentiment_score REAL;
ALTER TABLE messages      ADD COLUMN IF NOT EXISTS sentiment_label VARCHAR(16);
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS last_inbound_sentiment_score REAL;
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS last_inbound_sentiment_label VARCHAR(16);

-- Phase 3: normalized intent tags, one row per (conversation, tag). Source is
-- 'ai' (classifier) or 'manual' (user). AI re-classification must not clobber a
-- manual tag (enforced in the upsert's WHERE clause, not here).
CREATE TABLE IF NOT EXISTS conversation_tags (
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    tag             VARCHAR(32) NOT NULL,
    source          VARCHAR(8)  NOT NULL DEFAULT 'ai',
    confidence      REAL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, tag)
);

-- Faceted filtering: "conversations having tag IN (...)" and sentiment-level facets.
CREATE INDEX IF NOT EXISTS idx_conversation_tags_tag ON conversation_tags (tag);
CREATE INDEX IF NOT EXISTS idx_conversations_sentiment_label ON conversations (last_inbound_sentiment_label);
