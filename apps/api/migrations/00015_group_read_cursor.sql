-- +goose Up
-- Each member's read cursor for a group: the last message they have seen.
--
-- The cursor is a message id, not a timestamp: matching the chat history
-- cursor (created_at, id), it is exact and never drifts with the clock. The
-- created_at of that message rides along so unread counts can compare the
-- pair without a join back to chat_messages on every read.
ALTER TABLE group_members
    ADD COLUMN last_read_message_id         uuid REFERENCES chat_messages (id) ON DELETE SET NULL,
    ADD COLUMN last_read_message_created_at timestamptz;

-- Both columns are set together or not at all: a member who has never marked
-- anything read has no cursor, not a half-formed one.
ALTER TABLE group_members
    ADD CONSTRAINT group_members_last_read_consistent
    CHECK ((last_read_message_id IS NULL) = (last_read_message_created_at IS NULL));

-- +goose Down
ALTER TABLE group_members
    DROP CONSTRAINT group_members_last_read_consistent;

ALTER TABLE group_members
    DROP COLUMN last_read_message_created_at,
    DROP COLUMN last_read_message_id;
