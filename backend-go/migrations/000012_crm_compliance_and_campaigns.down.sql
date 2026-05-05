DROP INDEX IF EXISTS ix_messages_campaign;
ALTER TABLE messages       DROP CONSTRAINT IF EXISTS fk_messages_campaign;
ALTER TABLE conversations  DROP CONSTRAINT IF EXISTS fk_conversations_campaign;

DROP INDEX IF EXISTS ix_campaign_contacts_due;
DROP INDEX IF EXISTS ix_campaign_contacts_pending;
DROP INDEX IF EXISTS ix_campaign_contacts_status;
DROP TABLE IF EXISTS campaign_contacts;

DROP INDEX IF EXISTS ix_campaigns_user_status;
DROP TABLE IF EXISTS campaigns;

DROP INDEX IF EXISTS ix_contacts_unsubscribed;
ALTER TABLE contacts        DROP COLUMN IF EXISTS unsubscribe_reason;
ALTER TABLE contacts        DROP COLUMN IF EXISTS unsubscribed_at;

ALTER TABLE sender_profiles DROP COLUMN IF EXISTS physical_address;

ALTER TABLE user_channels   DROP COLUMN IF EXISTS unsubscribe_secret;
