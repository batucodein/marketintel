ALTER TABLE campaigns DROP COLUMN IF EXISTS on_negative_action;
ALTER TABLE campaigns DROP COLUMN IF EXISTS on_positive_action;
ALTER TABLE messages DROP COLUMN IF EXISTS sentiment;
ALTER TABLE conversations DROP COLUMN IF EXISTS last_inbound_sentiment;
