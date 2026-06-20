DROP INDEX IF EXISTS ix_campaigns_contact_group_id;
ALTER TABLE campaigns DROP COLUMN IF EXISTS contact_group_id;
DROP TABLE IF EXISTS contact_group_members;
DROP TABLE IF EXISTS contact_groups;
