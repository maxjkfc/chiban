-- +goose Up
-- Note: the reply FK below was changed from ON DELETE CASCADE to SET NULL after
-- this file was first written. Editing an applied migration is normally wrong —
-- goose_db_version records only a version number, no checksum, so an
-- environment that already ran 00008 would silently keep the old constraint.
-- This one had only ever been applied to one developer's dev and test
-- databases, both re-run and verified, and had not been merged or deployed.
-- Anything after this point in history gets a new migration instead.
-- A reply is an ordinary message pointing at another one. V0.1 has no separate
-- comment domain: a comment on a meal and a reply in chat are the same thing,
-- so the thread structure lives on the message itself.
-- SET NULL, never CASCADE: a reply is its author's own contribution and must
-- survive whatever happens to what it answered. Deletion in the API is soft, so
-- this only fires if a row is ever removed for real — a deleted account, say —
-- and even then the reply stays, minus its quote.
ALTER TABLE chat_messages
    ADD COLUMN reply_to_message_id uuid REFERENCES chat_messages (id) ON DELETE SET NULL;

CREATE TABLE message_reactions (
    message_id uuid NOT NULL REFERENCES chat_messages (id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Which emoji are offered is a product decision that changes without a
    -- schema change, so the set lives in the application. The length bound is
    -- here to keep the column from becoming a place to put anything at all.
    reaction_type text NOT NULL CHECK (length(reaction_type) BETWEEN 1 AND 16),
    created_at    timestamptz NOT NULL DEFAULT now(),
    -- One person, one message, one kind. This is what makes a repeated tap
    -- impossible to count twice, rather than the application remembering to
    -- check first.
    PRIMARY KEY (message_id, user_id, reaction_type)
);

-- +goose Down
DROP TABLE message_reactions;

ALTER TABLE chat_messages
    DROP COLUMN reply_to_message_id;
