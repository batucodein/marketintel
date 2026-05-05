ALTER TABLE sender_profiles
    DROP COLUMN IF EXISTS catalog_file_name,
    DROP COLUMN IF EXISTS catalog_mime_type,
    DROP COLUMN IF EXISTS catalog_data,
    DROP COLUMN IF EXISTS catalog_size_bytes,
    DROP COLUMN IF EXISTS catalog_uploaded_at;
