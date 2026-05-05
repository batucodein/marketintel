-- P3 — Follow-up sequences. A sequence is a playbook of timed steps; a
-- sequence_run tracks one conversation's progress through that playbook.

CREATE TABLE sequences (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    is_template   BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_sequences_user ON sequences (user_id);

CREATE TABLE sequence_steps (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sequence_id     UUID NOT NULL REFERENCES sequences(id) ON DELETE CASCADE,
    step_number     INTEGER NOT NULL,
    wait_days       INTEGER NOT NULL DEFAULT 3,
    trigger         VARCHAR(32) NOT NULL DEFAULT 'no_reply',
    -- 'no_reply' | 'any_reply' | 'positive_reply' | 'always'
    action          VARCHAR(32) NOT NULL DEFAULT 'send_message',
    -- 'send_message' | 'mark_cold' | 'notify_user' | 'advance_stage'
    prompt_override TEXT,
    auto_send       BOOLEAN NOT NULL DEFAULT false,
    CONSTRAINT uq_sequence_step_number UNIQUE (sequence_id, step_number)
);
CREATE INDEX ix_sequence_steps_seq ON sequence_steps (sequence_id, step_number);

-- One row per (sequence, conversation). Tracks where the run is.
CREATE TABLE sequence_runs (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    sequence_id      UUID NOT NULL REFERENCES sequences(id) ON DELETE CASCADE,
    conversation_id  UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    current_step     INTEGER NOT NULL DEFAULT 1,
    next_run_at      TIMESTAMPTZ NOT NULL,
    status           VARCHAR(16) NOT NULL DEFAULT 'active',
    -- 'active' | 'paused' | 'completed' | 'stopped_on_reply'
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_sequence_run_conv UNIQUE (sequence_id, conversation_id)
);
-- Engine picks up due rows efficiently.
CREATE INDEX ix_sequence_runs_due ON sequence_runs (next_run_at)
    WHERE status = 'active';
CREATE INDEX ix_sequence_runs_conv ON sequence_runs (conversation_id);

-- Activate FKs that were placeholders.
ALTER TABLE messages
    ADD CONSTRAINT fk_messages_sequence_step
    FOREIGN KEY (sequence_step_id) REFERENCES sequence_steps(id) ON DELETE SET NULL;

ALTER TABLE contacts
    ADD CONSTRAINT fk_contacts_default_sequence
    FOREIGN KEY (default_sequence_id) REFERENCES sequences(id) ON DELETE SET NULL;

ALTER TABLE campaigns
    ADD CONSTRAINT fk_campaigns_sequence
    FOREIGN KEY (sequence_id) REFERENCES sequences(id) ON DELETE SET NULL;
