ALTER TABLE conversations
    DROP CONSTRAINT IF EXISTS conversations_channel_id_fkey;

ALTER TABLE conversations
    ADD CONSTRAINT conversations_channel_id_fkey
    FOREIGN KEY (channel_id) REFERENCES user_channels(id) ON DELETE CASCADE;
