-- +goose Up
-- A user's own sticker library. Same shape as chat_media: the binary lives in
-- object storage and this table holds only the pointer, so the browser never
-- sees a bucket or an object name.
CREATE TABLE user_stickers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    media_type   text NOT NULL CHECK (media_type IN ('image', 'gif')),
    bucket       text        NOT NULL,
    object_name  text        NOT NULL,
    content_type text        NOT NULL,
    size_bytes   bigint      NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    -- Deleting a sticker takes it out of the owner's picker. It does not
    -- retract the messages already sent with it, so the row and its object
    -- stay reachable and old conversations keep rendering.
    deleted_at   timestamptz
);

-- The picker asks for one user's live stickers, newest first.
CREATE INDEX user_stickers_owner_idx
    ON user_stickers (user_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- A sticker message carries a reference and nothing else; message_type
-- 'sticker' has been allowed since 00007.
ALTER TABLE chat_messages
    ADD COLUMN sticker_id uuid REFERENCES user_stickers (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE chat_messages
    DROP COLUMN sticker_id;

DROP INDEX user_stickers_owner_idx;

DROP TABLE user_stickers;
