-- Restore the NOT NULL market link. Orphaned scores (market deleted) can't
-- satisfy it, so drop them first.
DELETE FROM lead_scores WHERE market_id IS NULL;
ALTER TABLE lead_scores DROP CONSTRAINT IF EXISTS lead_scores_market_id_fkey;
ALTER TABLE lead_scores
    ADD CONSTRAINT lead_scores_market_id_fkey
    FOREIGN KEY (market_id) REFERENCES markets(id);
ALTER TABLE lead_scores ALTER COLUMN market_id SET NOT NULL;
