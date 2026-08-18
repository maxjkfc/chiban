-- +goose Up
CREATE TABLE chat_messages (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id  uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The full V0.1 vocabulary. Later slices start producing the other kinds;
    -- this is the domain's enum, not a speculative column.
    message_type text NOT NULL CHECK (
        message_type IN ('text', 'meal', 'image', 'gif', 'sticker', 'system')
    ),
    content text CHECK (content IS NULL OR length(content) <= 2000),
    -- Client-generated, so a retry after a dropped response resolves to the
    -- message that already exists instead of posting it twice.
    client_message_id uuid        NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    -- Soft delete: a removed message keeps its place so replies to it survive.
    deleted_at        timestamptz,
    UNIQUE (user_id, client_message_id)
);

-- Chat history is read newest-first in pages keyed on (created_at, id): the
-- pair is what makes the order total, since messages can share a timestamp.
CREATE INDEX chat_messages_group_cursor_idx
    ON chat_messages (group_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE chat_messages;
