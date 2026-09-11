-- +goose Up
-- A member's personal choice to keep a group at the top of their own list.
--
-- This is per-user state, not a group property: two members of the same
-- group can pin it differently, and it has to survive a login from a new
-- device, which is exactly why it lives here instead of client storage.
CREATE TABLE group_pins (
    user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    group_id  uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    pinned_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, group_id)
);

-- Unpinning and re-listing a member's pins both filter by user_id alone.
CREATE INDEX group_pins_user_id_idx ON group_pins (user_id);

-- +goose Down
DROP TABLE group_pins;
