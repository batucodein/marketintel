-- Move shipment_data back into social_links
UPDATE businesses
SET social_links = COALESCE(social_links, '{}'::jsonb) || jsonb_build_object('shipment_data', shipment_data)
WHERE shipment_data IS NOT NULL;

ALTER TABLE businesses DROP COLUMN IF EXISTS shipment_data;
