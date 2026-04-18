-- Replace regular index with unique index on google_place_id (needed for ON CONFLICT)
DROP INDEX IF EXISTS ix_businesses_google_place_id;
CREATE UNIQUE INDEX ix_businesses_google_place_id ON businesses (google_place_id) WHERE google_place_id IS NOT NULL;
