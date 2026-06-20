DROP TABLE IF EXISTS simulation_assistant_messages;

ALTER TABLE simulation_leads
    DROP COLUMN IF EXISTS state,
    DROP COLUMN IF EXISTS current_step,
    DROP COLUMN IF EXISTS touch,
    DROP COLUMN IF EXISTS last_direction,
    DROP COLUMN IF EXISTS last_out_day,
    DROP COLUMN IF EXISTS last_sentiment,
    DROP COLUMN IF EXISTS next_day,
    DROP COLUMN IF EXISTS snooze_until_day,
    DROP COLUMN IF EXISTS replied_once,
    DROP COLUMN IF EXISTS pending_draft,
    DROP COLUMN IF EXISTS resume_tags,
    DROP COLUMN IF EXISTS resume_sentiment,
    DROP COLUMN IF EXISTS prompt_ctx;

ALTER TABLE simulations
    DROP COLUMN IF EXISTS mode,
    DROP COLUMN IF EXISTS virtual_day,
    DROP COLUMN IF EXISTS playbook,
    DROP COLUMN IF EXISTS on_positive_action,
    DROP COLUMN IF EXISTS on_negative_action,
    DROP COLUMN IF EXISTS include_lessons;
