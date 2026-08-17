-- +goose Up
-- A profile exists only once onboarding has been completed, so its presence is
-- what tells the frontend whether to send someone to the onboarding flow.
CREATE TABLE profiles (
    user_id      uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    display_name text        NOT NULL CHECK (btrim(display_name) <> '' AND length(display_name) <= 50),
    -- IANA name, e.g. Asia/Taipei. Timestamps stay UTC; this is what turns
    -- them into the user's own "today".
    timezone     text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE profiles;
