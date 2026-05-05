-- One catalog per user, attached opt-in to outbound emails.
-- Stored inline in Postgres (BYTEA, capped at 10 MB in the handler).
-- Swap for Supabase/GCS storage later without changing the external API —
-- only catalog_data would be replaced by a catalog_storage_url column.
ALTER TABLE sender_profiles
    ADD COLUMN IF NOT EXISTS catalog_file_name  TEXT,
    ADD COLUMN IF NOT EXISTS catalog_mime_type  TEXT,
    ADD COLUMN IF NOT EXISTS catalog_data       BYTEA,
    ADD COLUMN IF NOT EXISTS catalog_size_bytes INTEGER,
    ADD COLUMN IF NOT EXISTS catalog_uploaded_at TIMESTAMPTZ;
