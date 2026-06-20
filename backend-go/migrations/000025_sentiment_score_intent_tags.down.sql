DROP INDEX IF EXISTS idx_conversations_sentiment_label;
DROP INDEX IF EXISTS idx_conversation_tags_tag;
DROP TABLE IF EXISTS conversation_tags;
ALTER TABLE conversations DROP COLUMN IF EXISTS last_inbound_sentiment_label;
ALTER TABLE conversations DROP COLUMN IF EXISTS last_inbound_sentiment_score;
ALTER TABLE messages DROP COLUMN IF EXISTS sentiment_label;
ALTER TABLE messages DROP COLUMN IF EXISTS sentiment_score;
