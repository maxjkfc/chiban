-- +goose Up
-- citext backs case-insensitive email uniqueness in the users table (slice 1).
-- Enforcing it in the database rather than by lowercasing in application code
-- keeps the invariant true regardless of which code path inserts the row.
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
