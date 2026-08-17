package group

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DBTX is the slice of database/sql this package needs, so the same queries
// run against a connection or a transaction.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type store struct {
	db DBTX
}

func (s *store) createGroup(ctx context.Context, ownerID uuid.UUID, name string) (Group, error) {
	var g Group
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO groups (name, owner_id)
		VALUES ($1, $2)
		RETURNING id, name, owner_id, created_at
	`, name, ownerID).Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt)
	if err != nil {
		return Group{}, fmt.Errorf("group: create: %w", err)
	}
	return g, nil
}

// addMember is idempotent: re-using an invite link must not fail, and must not
// demote an owner to member either.
func (s *store) addMember(ctx context.Context, groupID, userID uuid.UUID, role string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO group_members (group_id, user_id, role)
		VALUES ($1, $2, $3)
		ON CONFLICT (group_id, user_id) DO NOTHING
	`, groupID, userID, role)
	if err != nil {
		return fmt.Errorf("group: add member: %w", err)
	}
	return nil
}

func (s *store) removeMember(ctx context.Context, groupID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return fmt.Errorf("group: remove member: %w", err)
	}
	return nil
}

// findForMember is the authorization check for every group read: a
// non-member's query returns nothing, which becomes ErrNotMember.
func (s *store) findForMember(ctx context.Context, groupID, userID uuid.UUID) (Group, error) {
	var g Group
	err := s.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.owner_id, m.role, g.created_at
		FROM groups g
		JOIN group_members m ON m.group_id = g.id AND m.user_id = $2
		WHERE g.id = $1
	`, groupID, userID).Scan(&g.ID, &g.Name, &g.OwnerID, &g.Role, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Group{}, ErrNotMember
	}
	if err != nil {
		return Group{}, fmt.Errorf("group: find: %w", err)
	}
	return g, nil
}

func (s *store) listForUser(ctx context.Context, userID uuid.UUID) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT g.id, g.name, g.owner_id, m.role, g.created_at
		FROM groups g
		JOIN group_members m ON m.group_id = g.id
		WHERE m.user_id = $1
		ORDER BY g.created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("group: list: %w", err)
	}
	defer rows.Close()

	groups := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerID, &g.Role, &g.CreatedAt); err != nil {
			return nil, fmt.Errorf("group: scan: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *store) listMembers(ctx context.Context, groupID uuid.UUID) ([]Member, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, role, joined_at
		FROM group_members
		WHERE group_id = $1
		ORDER BY joined_at
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("group: list members: %w", err)
	}
	defer rows.Close()

	members := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("group: scan member: %w", err)
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (s *store) createInvite(ctx context.Context, groupID, createdBy uuid.UUID, code string, expiresAt time.Time) (Invite, error) {
	var i Invite
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO group_invites (group_id, code, created_by, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, group_id, code, expires_at
	`, groupID, code, createdBy, expiresAt).Scan(&i.ID, &i.GroupID, &i.Code, &i.ExpiresAt)
	if err != nil {
		return Invite{}, fmt.Errorf("group: create invite: %w", err)
	}
	return i, nil
}

// findUsableInvite applies expiry and revocation in the query, so no caller can
// forget to check them.
func (s *store) findUsableInvite(ctx context.Context, code string, now time.Time) (Invite, error) {
	var i Invite
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, code, expires_at
		FROM group_invites
		WHERE code = $1 AND revoked_at IS NULL AND expires_at > $2
	`, code, now).Scan(&i.ID, &i.GroupID, &i.Code, &i.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrInviteInvalid
	}
	if err != nil {
		return Invite{}, fmt.Errorf("group: find invite: %w", err)
	}
	return i, nil
}

func (s *store) revokeInvite(ctx context.Context, groupID, inviteID uuid.UUID, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE group_invites
		SET revoked_at = $3
		WHERE id = $1 AND group_id = $2 AND revoked_at IS NULL
	`, inviteID, groupID, now)
	if err != nil {
		return fmt.Errorf("group: revoke invite: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("group: revoke invite: %w", err)
	}
	if affected == 0 {
		return ErrInviteInvalid
	}
	return nil
}
