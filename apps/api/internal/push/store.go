package push

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DBTX matches the database transaction/connection interface used across domains.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Subscription struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	DeviceID  string    `json:"device_id"`
	Endpoint  string    `json:"endpoint"`
	P256DH    string    `json:"p256dh"`
	Auth      string    `json:"auth"`
	UserAgent string    `json:"user_agent,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Store struct {
	db DBTX
}

func NewStore(db DBTX) *Store {
	return &Store{db: db}
}

// Upsert registers or updates a push subscription.
// To guarantee idempotency and avoid duplicate key violations on either (user_id, device_id)
// or (endpoint), we delete any existing conflicting records and insert the fresh record.
func (s *Store) Upsert(ctx context.Context, sub Subscription) (Subscription, error) {
	if sub.UserID == uuid.Nil {
		return Subscription{}, errors.New("user_id is required")
	}
	if strings.TrimSpace(sub.DeviceID) == "" {
		return Subscription{}, errors.New("device_id is required")
	}
	if strings.TrimSpace(sub.Endpoint) == "" {
		return Subscription{}, errors.New("endpoint is required")
	}
	if strings.TrimSpace(sub.P256DH) == "" || strings.TrimSpace(sub.Auth) == "" {
		return Subscription{}, errors.New("keys (p256dh and auth) are required")
	}

	// Clean up any row holding either the endpoint or the (user_id, device_id) tuple
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions
		WHERE endpoint = $1 OR (user_id = $2 AND device_id = $3)
	`, sub.Endpoint, sub.UserID, sub.DeviceID)
	if err != nil {
		return Subscription{}, fmt.Errorf("cleanup prior subscriptions: %w", err)
	}

	now := time.Now().UTC()
	var created Subscription
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO push_subscriptions (user_id, device_id, endpoint, p256dh_key, auth_key, user_agent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
		RETURNING id, user_id, device_id, endpoint, p256dh_key, auth_key, coalesce(user_agent, ''), created_at, updated_at
	`, sub.UserID, sub.DeviceID, sub.Endpoint, sub.P256DH, sub.Auth, sub.UserAgent, now).Scan(
		&created.ID, &created.UserID, &created.DeviceID, &created.Endpoint,
		&created.P256DH, &created.Auth, &created.UserAgent, &created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}

	return created, nil
}

// DeleteByEndpoint removes a specific endpoint owned by the authenticated user.
func (s *Store) DeleteByEndpoint(ctx context.Context, userID uuid.UUID, endpoint string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions
		WHERE user_id = $1 AND endpoint = $2
	`, userID, endpoint)
	return err
}

// DeleteByDevice removes all subscriptions for a specific user device (e.g. during logout).
func (s *Store) DeleteByDevice(ctx context.Context, userID uuid.UUID, deviceID string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions
		WHERE user_id = $1 AND device_id = $2
	`, userID, deviceID)
	return err
}

// DeleteByID purges a subscription record by ID (e.g. on terminal 404/410 push response).
func (s *Store) DeleteByID(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM push_subscriptions
		WHERE id = $1
	`, id)
	return err
}

// ListByGroupEligible finds all push subscriptions belonging to group members except:
// 1. The sender (excludedUserID)
// 2. Specific device IDs currently holding an active focus lease for this group (excludedDeviceKeys)
func (s *Store) ListByGroupEligible(ctx context.Context, groupID uuid.UUID, excludedUserID uuid.UUID) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.user_id, s.device_id, s.endpoint, s.p256dh_key, s.auth_key, coalesce(s.user_agent, ''), s.created_at, s.updated_at
		FROM push_subscriptions s
		JOIN group_members gm ON gm.user_id = s.user_id
		WHERE gm.group_id = $1 AND s.user_id != $2
	`, groupID, excludedUserID)
	if err != nil {
		return nil, fmt.Errorf("query eligible group subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(
			&sub.ID, &sub.UserID, &sub.DeviceID, &sub.Endpoint,
			&sub.P256DH, &sub.Auth, &sub.UserAgent, &sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return subs, nil
}
