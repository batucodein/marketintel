ALTER TABLE ai_request_log ADD COLUMN search_id UUID REFERENCES searches(id);
CREATE INDEX idx_ai_request_log_user_id ON ai_request_log(user_id);
CREATE INDEX idx_ai_request_log_search_id ON ai_request_log(search_id);
CREATE INDEX idx_ai_request_log_created_at ON ai_request_log(created_at);
