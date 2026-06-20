-- Contact groups: named, user-owned collections of contacts with their own
-- brand. Created when promoting market leads to contacts; email groups are
-- built from a contact group (inheriting its brand + members). Contacts stay
-- one-per-(user,business) globally — membership is many-to-many, so dedup is
-- preserved and a contact can live in several groups.

CREATE TABLE contact_groups (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    sender_profile_id UUID REFERENCES sender_profiles(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_contact_groups_user ON contact_groups (user_id);

CREATE TABLE contact_group_members (
    contact_group_id UUID NOT NULL REFERENCES contact_groups(id) ON DELETE CASCADE,
    contact_id       UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    added_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (contact_group_id, contact_id)
);
CREATE INDEX ix_contact_group_members_contact ON contact_group_members (contact_id);

-- An email group (campaign) is built from a contact group.
ALTER TABLE campaigns ADD COLUMN contact_group_id UUID REFERENCES contact_groups(id) ON DELETE SET NULL;
CREATE INDEX ix_campaigns_contact_group_id ON campaigns (contact_group_id);
