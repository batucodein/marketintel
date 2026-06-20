-- Reply sentiment + response-aware follow-ups.
-- Persist the sentiment of every inbound reply (so we can show it + branch on
-- it), and give each campaign (email group) a positive/negative branch action.

ALTER TABLE conversations ADD COLUMN IF NOT EXISTS last_inbound_sentiment VARCHAR(16);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS sentiment VARCHAR(16);

ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS on_positive_action VARCHAR(24) NOT NULL DEFAULT 'auto_draft_reply';
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS on_negative_action VARCHAR(24) NOT NULL DEFAULT 'mark_cold';
