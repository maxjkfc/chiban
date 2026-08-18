-- +goose Up
-- Uploaded chat images and GIFs. Binary data lives in object storage; this
-- table holds only the pointer, so the browser never sees a bucket or an
-- object name.
CREATE TABLE chat_media (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    media_type   text NOT NULL CHECK (media_type IN ('image', 'gif')),
    bucket       text        NOT NULL,
    object_name  text        NOT NULL,
    content_type text        NOT NULL,
    size_bytes   bigint      NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    deleted_at   timestamptz
);

-- A message points at its media; the media carries nothing about the message,
-- so who may read it is decided by where it was posted rather than by the
-- upload itself.
ALTER TABLE chat_messages
    ADD COLUMN chat_media_id uuid REFERENCES chat_media (id) ON DELETE SET NULL;

-- Reading media asks "is this attached to a message in a group I am in", which
-- runs from the media side.
CREATE INDEX chat_messages_media_idx
    ON chat_messages (chat_media_id)
    WHERE chat_media_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX chat_messages_media_idx;

ALTER TABLE chat_messages
    DROP COLUMN chat_media_id;

DROP TABLE chat_media;
