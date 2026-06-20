-- Simulation Lab: AI-persona test runs of the outreach engine. Self-contained
-- — these tables never touch campaigns/conversations/messages and no email is
-- ever sent. They store the run config + per-lead transcripts + grades.

CREATE TABLE simulations (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              TEXT NOT NULL DEFAULT '',
    market_id         UUID REFERENCES markets(id) ON DELETE SET NULL,
    sender_profile_id UUID REFERENCES sender_profiles(id) ON DELETE SET NULL,
    steps             JSONB NOT NULL DEFAULT '[]',
    persona_config    JSONB NOT NULL DEFAULT '{}',
    status            VARCHAR(16) NOT NULL DEFAULT 'running', -- running | done | failed
    summary           JSONB,
    error             TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ
);
CREATE INDEX ix_simulations_user ON simulations (user_id, created_at DESC);

CREATE TABLE simulation_leads (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    simulation_id UUID NOT NULL REFERENCES simulations(id) ON DELETE CASCADE,
    persona       VARCHAR(32) NOT NULL,
    display_name  TEXT NOT NULL DEFAULT '',
    business_id   UUID,
    transcript    JSONB NOT NULL DEFAULT '[]', -- [{who, day, subject, body, sentiment}]
    outcome       VARCHAR(24) NOT NULL DEFAULT '',
    grade         JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_simulation_leads_sim ON simulation_leads (simulation_id);
