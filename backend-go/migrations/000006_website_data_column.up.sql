-- Add website_data column to store enriched website scraping results
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS website_data JSONB;

-- Add email_verified column to track SMTP verification
ALTER TABLE businesses ADD COLUMN IF NOT EXISTS email_verified BOOLEAN;
