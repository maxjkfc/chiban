-- +goose Up
-- 00015 pairs last_read_message_id with last_read_message_created_at and
-- requires both or neither to be NULL. But a hard delete of the referenced
-- chat_messages row only triggers the FK's ON DELETE SET NULL on
-- last_read_message_id itself: PostgreSQL's referential action can target
-- the FK column, never a sibling column in the same row, so
-- last_read_message_created_at is left behind and the delete fails
-- group_members_last_read_consistent.
--
-- A BEFORE DELETE trigger on chat_messages clears both columns together
-- before the row disappears, so by the time the delete (and the FK's own
-- SET NULL, now a no-op since nothing still references the row) actually
-- happens, the pair is already NULL/NULL and the CHECK is never violated.
-- This keeps the documented ON DELETE SET NULL contract: the cursor is
-- reset to "never read", not left half-formed or blocking the delete.
-- +goose StatementBegin
CREATE FUNCTION group_members_clear_read_cursor() RETURNS trigger AS $$
BEGIN
    UPDATE group_members
    SET last_read_message_id = NULL,
        last_read_message_created_at = NULL
    WHERE last_read_message_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER chat_messages_clear_read_cursor
    BEFORE DELETE ON chat_messages
    FOR EACH ROW
    EXECUTE FUNCTION group_members_clear_read_cursor();

-- +goose Down
DROP TRIGGER chat_messages_clear_read_cursor ON chat_messages;
DROP FUNCTION group_members_clear_read_cursor();
