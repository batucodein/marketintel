ALTER TABLE campaigns DROP CONSTRAINT IF EXISTS fk_campaigns_sequence;
ALTER TABLE contacts  DROP CONSTRAINT IF EXISTS fk_contacts_default_sequence;
ALTER TABLE messages  DROP CONSTRAINT IF EXISTS fk_messages_sequence_step;

DROP INDEX IF EXISTS ix_sequence_runs_conv;
DROP INDEX IF EXISTS ix_sequence_runs_due;
DROP TABLE IF EXISTS sequence_runs;

DROP INDEX IF EXISTS ix_sequence_steps_seq;
DROP TABLE IF EXISTS sequence_steps;

DROP INDEX IF EXISTS ix_sequences_user;
DROP TABLE IF EXISTS sequences;
