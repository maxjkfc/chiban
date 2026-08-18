-- +goose Up
-- Sharing is an explicit, revocable relationship — never inferred from the
-- existence of a chat message. This table is the only source of truth for
-- whether a meal is visible to a group.
CREATE TABLE meal_group_shares (
    meal_record_id uuid        NOT NULL REFERENCES meal_records (id) ON DELETE CASCADE,
    group_id       uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    shared_at      timestamptz NOT NULL DEFAULT now(),
    -- Revoking keeps the row: re-sharing later must not be blocked by the
    -- unique key, and the row records that this group once had access.
    revoked_at     timestamptz,
    PRIMARY KEY (meal_record_id, group_id)
);

-- Reading a meal asks "is it shared with any group I am in", so the lookup
-- runs from the group side.
CREATE INDEX meal_group_shares_group_idx
    ON meal_group_shares (group_id)
    WHERE revoked_at IS NULL;

-- A meal message carries a reference and nothing else: no description, no
-- photo paths, no future nutrition fields. The card is assembled at read time
-- from the meal itself, so it can never drift from — or outlive — the record.
--
-- SET NULL rather than CASCADE: if a meal row is ever removed for real, the
-- conversation around it stays. A meal message without a reference reads as
-- deleted, which is the same thing the client shows when the meal is soft
-- deleted and the fetch comes back empty.
ALTER TABLE chat_messages
    ADD COLUMN meal_record_id uuid REFERENCES meal_records (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE chat_messages
    DROP COLUMN meal_record_id;

DROP TABLE meal_group_shares;
