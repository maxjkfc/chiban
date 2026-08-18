-- +goose Up
-- One card per meal per group, enforced by the database rather than by the
-- application checking before it writes. Two shares racing each other — a
-- retry, a double tap — could both find no card and both post one.
--
-- Partial, on the live rows only: a deleted card must not stop the meal from
-- being shared into that group again.
CREATE UNIQUE INDEX chat_messages_one_meal_card_per_group_idx
    ON chat_messages (group_id, meal_record_id)
    WHERE meal_record_id IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX chat_messages_one_meal_card_per_group_idx;
