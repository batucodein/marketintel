-- Bump ai_prompt_version to VARCHAR(64) so prompt names like
-- 'outreach_draft_v1' (17 chars) fit. Original 16 was too tight.
ALTER TABLE messages ALTER COLUMN ai_prompt_version TYPE VARCHAR(64);
