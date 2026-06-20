DROP INDEX IF EXISTS ix_markets_sender_profile_id;
ALTER TABLE markets DROP COLUMN IF EXISTS sender_profile_id;
