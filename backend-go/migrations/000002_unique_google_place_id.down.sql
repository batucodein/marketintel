DROP INDEX IF EXISTS ix_businesses_google_place_id;
CREATE INDEX ix_businesses_google_place_id ON businesses (google_place_id);
