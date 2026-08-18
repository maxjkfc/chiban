-- +goose Up
CREATE TABLE groups (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text        NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 50),
    -- A group always has an owner: V0.1 refuses to let the owner leave rather
    -- than allowing an ownerless group to exist.
    owner_id   uuid        NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Membership is the basis of every group authorization check, so the database
-- guarantees a user appears at most once per group.
CREATE TABLE group_members (
    group_id  uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role      text        NOT NULL CHECK (role IN ('owner', 'member')),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX group_members_user_id_idx ON group_members (user_id);

CREATE TABLE group_invites (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   uuid        NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    code       text        NOT NULL UNIQUE,
    created_by uuid        NOT NULL REFERENCES users (id),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX group_invites_group_id_idx ON group_invites (group_id);

-- +goose Down
DROP TABLE group_invites;
DROP TABLE group_members;
DROP TABLE groups;
