-- +goose Up
-- Push subscriptions for browser / PWA Web Push (VAPID).
--
-- Each subscription represents a registered browser endpoint belonging to a
-- device and user. When an endpoint or device registers again, the store updates
-- or replaces the row idempotently.
CREATE TABLE push_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh_key TEXT NOT NULL,
    auth_key TEXT NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT push_subscriptions_user_device_unique UNIQUE (user_id, device_id)
);

CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions(user_id);

-- +goose Down
DROP TABLE push_subscriptions;
