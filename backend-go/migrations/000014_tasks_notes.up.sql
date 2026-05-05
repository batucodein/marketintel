-- P4 — CRM extras. Tasks (with optional contact/conversation link) and
-- free-text notes pinned to a contact.

CREATE TABLE tasks (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id      UUID REFERENCES contacts(id) ON DELETE CASCADE,
    conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    due_at          TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Open tasks ordered by due date (NULL last) for the overdue/today/upcoming view.
CREATE INDEX ix_tasks_open_due ON tasks (user_id, due_at NULLS LAST)
    WHERE completed_at IS NULL;
CREATE INDEX ix_tasks_contact ON tasks (contact_id) WHERE contact_id IS NOT NULL;

CREATE TABLE notes (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id  UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
    body        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_notes_contact ON notes (contact_id, created_at DESC);
