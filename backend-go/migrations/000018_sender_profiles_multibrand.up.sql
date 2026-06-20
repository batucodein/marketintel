-- Multi-brand sender profiles.
-- Today sender_profiles is one-per-user (PK = user_id). To let a user reach
-- different markets as different brands, switch to many-per-user: a surrogate
-- id PK + a brand name, with user_id demoted to a non-unique owner FK.
-- The existing single row per user is preserved as their "Default" brand.

ALTER TABLE sender_profiles ADD COLUMN IF NOT EXISTS id UUID;
UPDATE sender_profiles SET id = uuid_generate_v4() WHERE id IS NULL;
ALTER TABLE sender_profiles ALTER COLUMN id SET NOT NULL;
ALTER TABLE sender_profiles ALTER COLUMN id SET DEFAULT uuid_generate_v4();

ALTER TABLE sender_profiles ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT 'Default';

-- Swap the primary key from user_id to id.
ALTER TABLE sender_profiles DROP CONSTRAINT sender_profiles_pkey;
ALTER TABLE sender_profiles ADD PRIMARY KEY (id);

CREATE INDEX IF NOT EXISTS ix_sender_profiles_user_id ON sender_profiles (user_id);
