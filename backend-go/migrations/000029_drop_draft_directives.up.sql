-- Retire the standing-directives concept: the draft assistant's only memory is
-- now the playbook (campaign_tag_guidance). Cold-email rules are one-time edits.
DROP INDEX IF EXISTS idx_campaign_draft_directives;
DROP TABLE IF EXISTS campaign_draft_directives;
