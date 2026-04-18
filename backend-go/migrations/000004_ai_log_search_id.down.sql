DROP INDEX IF EXISTS idx_ai_request_log_created_at;
DROP INDEX IF EXISTS idx_ai_request_log_search_id;
DROP INDEX IF EXISTS idx_ai_request_log_user_id;
ALTER TABLE ai_request_log DROP COLUMN IF EXISTS search_id;
