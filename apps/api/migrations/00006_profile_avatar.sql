-- +goose Up
-- The avatar's storage location lives on the profile rather than in a table of
-- its own: a profile has exactly one, and V0.1 has no other consumer of a
-- generic media record. Chat media and stickers get their own tables when they
-- arrive, because those are many-per-user and referenced from messages.
ALTER TABLE profiles
    ADD COLUMN avatar_media_id    uuid,
    -- The client only ever sees avatar_media_id; these two stay server-side.
    ADD COLUMN avatar_bucket      text,
    ADD COLUMN avatar_object_name text,
    ADD COLUMN avatar_content_type text,
    -- Either the whole avatar is set or none of it is.
    ADD CONSTRAINT profiles_avatar_complete CHECK (
        num_nonnulls(avatar_media_id, avatar_bucket, avatar_object_name, avatar_content_type) IN (0, 4)
    );

-- Reading an avatar starts from the media ID the client was given.
CREATE UNIQUE INDEX profiles_avatar_media_id_idx ON profiles (avatar_media_id)
    WHERE avatar_media_id IS NOT NULL;

-- +goose Down
DROP INDEX profiles_avatar_media_id_idx;
ALTER TABLE profiles
    DROP CONSTRAINT profiles_avatar_complete,
    DROP COLUMN avatar_content_type,
    DROP COLUMN avatar_object_name,
    DROP COLUMN avatar_bucket,
    DROP COLUMN avatar_media_id;
