-- Lead scores must outlive their market. They're the AI's buyer-fit analysis
-- and the draft prompt looks them up by (business_id, user_id) — not market.
-- Make market_id nullable and SET NULL on market delete so deleting a market
-- never throws away a contact's score.
ALTER TABLE lead_scores ALTER COLUMN market_id DROP NOT NULL;
ALTER TABLE lead_scores DROP CONSTRAINT IF EXISTS lead_scores_market_id_fkey;
ALTER TABLE lead_scores
    ADD CONSTRAINT lead_scores_market_id_fkey
    FOREIGN KEY (market_id) REFERENCES markets(id) ON DELETE SET NULL;
