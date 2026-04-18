ALTER TABLE markets
    DROP COLUMN IF EXISTS derived_product_name,
    DROP COLUMN IF EXISTS dominant_hs_code,
    DROP COLUMN IF EXISTS all_hs_codes,
    DROP COLUMN IF EXISTS origin_country,
    DROP COLUMN IF EXISTS origin_countries,
    DROP COLUMN IF EXISTS origin_share,
    DROP COLUMN IF EXISTS shipment_from_date,
    DROP COLUMN IF EXISTS shipment_to_date,
    DROP COLUMN IF EXISTS importer_count,
    DROP COLUMN IF EXISTS shipment_count,
    DROP COLUMN IF EXISTS source_file_name,
    DROP COLUMN IF EXISTS uploaded_at;
