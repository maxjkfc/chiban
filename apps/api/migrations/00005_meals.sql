-- +goose Up
-- MealRecord is the product's primary data. It exists independently of chat:
-- a meal message references it, and deleting a meal never deletes the
-- conversation around it.
CREATE TABLE meal_records (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    meal_type   text CHECK (meal_type IN ('breakfast', 'lunch', 'dinner', 'snack', 'other')),
    eaten_at    timestamptz NOT NULL,
    description text CHECK (description IS NULL OR length(description) <= 500),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    -- Soft delete: a deleted meal disappears from the owner's history but the
    -- chat thread that discussed it keeps its structure.
    deleted_at  timestamptz
);

-- Today and history both read "this user's meals in a time range".
CREATE INDEX meal_records_user_eaten_idx
    ON meal_records (user_id, eaten_at DESC)
    WHERE deleted_at IS NULL;

-- Binary data lives in object storage; this table holds only the pointer.
CREATE TABLE meal_photos (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    meal_record_id uuid        NOT NULL REFERENCES meal_records (id) ON DELETE CASCADE,
    bucket         text        NOT NULL,
    object_name    text        NOT NULL,
    content_type   text        NOT NULL,
    size_bytes     bigint      NOT NULL,
    sort_order     smallint    NOT NULL CHECK (sort_order BETWEEN 0 AND 3),
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (meal_record_id, sort_order)
);

CREATE INDEX meal_photos_meal_idx ON meal_photos (meal_record_id, sort_order);

-- +goose Down
DROP TABLE meal_photos;
DROP TABLE meal_records;
