-- +goose Up
-- The stickers a person keeps on the chat composer's quick rail.
--
-- This is a choice, not a usage statistic: the owner picks which ones sit
-- above the keyboard, so it is a preference and belongs next to the sticker.
-- Keeping it in the browser instead would lose it on every new device, which
-- is exactly the case a chosen list has to survive.
ALTER TABLE user_stickers
    ADD COLUMN pin_order smallint CHECK (pin_order BETWEEN 1 AND 4);

-- One sticker per slot per person, enforced by the database rather than by
-- whoever writes the update. Deleting a sticker clears its slot, so the
-- deleted_at clause is belt-and-braces: it keeps a tombstone from holding a
-- slot even if some future path forgets to release it.
CREATE UNIQUE INDEX user_stickers_pin_slot_idx
    ON user_stickers (user_id, pin_order)
    WHERE pin_order IS NOT NULL AND deleted_at IS NULL;

-- +goose Down
DROP INDEX user_stickers_pin_slot_idx;

ALTER TABLE user_stickers
    DROP COLUMN pin_order;
