-- Add dedicated shipment_data column
ALTER TABLE businesses ADD COLUMN shipment_data JSONB;

-- Migrate existing shipment data from social_links
UPDATE businesses
SET shipment_data = social_links->'shipment_data'
WHERE social_links IS NOT NULL
  AND social_links->'shipment_data' IS NOT NULL
  AND social_links->>'shipment_data' != 'null';

-- Clean social_links: remove shipment_data key, keep only actual social links
-- For rows that ONLY had shipment_data, set to null
UPDATE businesses
SET social_links = NULL
WHERE social_links IS NOT NULL
  AND social_links->'shipment_data' IS NOT NULL
  AND (social_links - 'shipment_data')::text IN ('{}', 'null');

-- For rows that had both shipment_data and other keys, remove just shipment_data
UPDATE businesses
SET social_links = social_links - 'shipment_data'
WHERE social_links IS NOT NULL
  AND social_links->'shipment_data' IS NOT NULL;
