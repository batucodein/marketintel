-- paused_at records WHEN an Email Group (campaign) was paused, so Resume can
-- shift the whole remaining schedule forward by the pause duration ("freeze &
-- thaw") instead of firing everything that became overdue while paused.
ALTER TABLE campaigns ADD COLUMN paused_at TIMESTAMPTZ;
