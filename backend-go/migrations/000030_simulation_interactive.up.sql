-- Interactive, step-by-step Simulation Lab: evolve the sim tables from a
-- finished-transcript store into LIVE, resumable state so the user can advance
-- the cadence step-by-step and intervene (approve/edit/refine drafts, chat with
-- the agent, manage the playbook). Still self-contained — no campaigns/
-- conversations/messages rows, no email ever sent.

ALTER TABLE simulations
    ADD COLUMN mode               VARCHAR(16)  NOT NULL DEFAULT 'interactive', -- interactive | quick
    ADD COLUMN virtual_day        INT          NOT NULL DEFAULT 0,
    ADD COLUMN playbook           JSONB        NOT NULL DEFAULT '{}',          -- tag -> instruction, editable mid-run
    ADD COLUMN on_positive_action VARCHAR(32)  NOT NULL DEFAULT '',
    ADD COLUMN on_negative_action VARCHAR(32)  NOT NULL DEFAULT '',
    ADD COLUMN include_lessons    BOOLEAN      NOT NULL DEFAULT true;
-- status gains 'paused' (awaiting user). Column is a plain VARCHAR (no enum) so
-- no constraint change is needed.

ALTER TABLE simulation_leads
    ADD COLUMN state            VARCHAR(20) NOT NULL DEFAULT 'done', -- active | awaiting_approval | snoozed | done
    ADD COLUMN current_step     INT         NOT NULL DEFAULT 0,      -- 0-based cadence step cursor
    ADD COLUMN touch            INT         NOT NULL DEFAULT 0,      -- index of last outbound (0=cold)
    ADD COLUMN last_direction   VARCHAR(4),                          -- out | in
    ADD COLUMN last_out_day     INT         NOT NULL DEFAULT 0,      -- virtual day of last outbound
    ADD COLUMN last_sentiment   VARCHAR(16),                         -- legacy sentiment of last inbound
    ADD COLUMN next_day         INT         NOT NULL DEFAULT 0,      -- virtual day this lead is next due
    ADD COLUMN snooze_until_day INT,                                 -- OOO resume day
    ADD COLUMN replied_once     BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN pending_draft    JSONB,                               -- {kind, subject, body, step} awaiting approval
    ADD COLUMN resume_tags      JSONB,                               -- remaining tags to re-route after snooze
    ADD COLUMN resume_sentiment VARCHAR(16),
    ADD COLUMN prompt_ctx       JSONB;                               -- cached prompt context (business/shipment/score/contact/persona)

-- Agent chat thread for a simulation (mirror of campaign_assistant_messages).
CREATE TABLE simulation_assistant_messages (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    simulation_id   UUID NOT NULL REFERENCES simulations(id) ON DELETE CASCADE,
    role            VARCHAR(16)  NOT NULL,                 -- user | assistant
    content         TEXT         NOT NULL DEFAULT '',
    proposed_action JSONB,
    status          VARCHAR(16)  NOT NULL DEFAULT 'sent',  -- sent | proposed | applied | dismissed
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX ix_sim_assistant_msgs ON simulation_assistant_messages (simulation_id, created_at);
