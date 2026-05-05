DROP TABLE IF EXISTS search_column_mappings;
ALTER TABLE lead_scores DROP COLUMN IF EXISTS dimension_completeness;
ALTER TABLE businesses  DROP COLUMN IF EXISTS input_field_presence;
