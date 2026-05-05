-- Defense-in-depth: stop CASCADE-deleting conversations when a user_channel
-- is removed. Past versions of the channel "Delete" action wiped every
-- prior conversation on that channel; we now soft-disable channels, but
-- the FK should also reject any rogue DELETE that bypasses the handler.
ALTER TABLE conversations
    DROP CONSTRAINT IF EXISTS conversations_channel_id_fkey;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_channel_id_fkey
    FOREIGN KEY (channel_id) REFERENCES user_channels(id) ON DELETE RESTRICT;
