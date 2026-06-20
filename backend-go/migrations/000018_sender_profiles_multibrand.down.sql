-- Collapse multi-brand back to one-per-user. Destructive: keeps only the
-- most-recently-updated profile per user.
DROP INDEX IF EXISTS ix_sender_profiles_user_id;

DELETE FROM sender_profiles a
 USING sender_profiles b
 WHERE a.user_id = b.user_id
   AND a.updated_at < b.updated_at;

ALTER TABLE sender_profiles DROP CONSTRAINT sender_profiles_pkey;
ALTER TABLE sender_profiles ADD PRIMARY KEY (user_id);
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS id;
ALTER TABLE sender_profiles DROP COLUMN IF EXISTS name;
