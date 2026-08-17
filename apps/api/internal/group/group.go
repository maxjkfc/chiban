// Package group owns private groups, their membership and their invites.
//
// Membership is the authorization basis for everything that happens inside a
// group — chat, shared meals, reactions — so every read and write here checks
// it against the session user rather than trusting anything from the client.
package group

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotMember is returned for groups the user cannot see. Handlers turn
	// it into 403; it deliberately does not distinguish "no such group" from
	// "not your group".
	ErrNotMember = errors.New("group: not a member")
	// ErrNotOwner covers owner-only operations such as revoking an invite.
	ErrNotOwner = errors.New("group: not the owner")
	// ErrOwnerCannotLeave keeps a group from ending up without an owner.
	// V0.1 has no ownership transfer, so the owner simply stays.
	ErrOwnerCannotLeave = errors.New("group: owner cannot leave the group")
	// ErrInviteInvalid covers unknown, expired and revoked invite codes alike.
	ErrInviteInvalid = errors.New("group: invite is not valid")
)

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("group: invalid %s: %s", e.Field, e.Message)
}

// InviteLifetime is how long a new invite stays usable. The same invite can be
// used by several people until it expires or is revoked.
const InviteLifetime = 7 * 24 * time.Hour

const (
	RoleOwner  = "owner"
	RoleMember = "member"

	maxNameLength = 50
)

type Group struct {
	ID        uuid.UUID
	Name      string
	OwnerID   uuid.UUID
	Role      string
	CreatedAt time.Time
}

type Member struct {
	UserID   uuid.UUID
	Role     string
	JoinedAt time.Time
}

type Invite struct {
	ID        uuid.UUID
	GroupID   uuid.UUID
	Code      string
	ExpiresAt time.Time
}

type Service struct {
	db    *sql.DB
	store *store
	now   func() time.Time
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, store: &store{db: db}, now: time.Now}
}

// Create makes the group and its owner membership in one transaction: a group
// whose owner is not a member would fail every later authorization check.
func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, name string) (Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Group{}, InvalidInputError{Field: "name", Message: "is required"}
	}
	if len([]rune(name)) > maxNameLength {
		return Group{}, InvalidInputError{
			Field:   "name",
			Message: fmt.Sprintf("must be at most %d characters", maxNameLength),
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, fmt.Errorf("group: begin: %w", err)
	}
	defer tx.Rollback()

	txStore := &store{db: tx}
	g, err := txStore.createGroup(ctx, ownerID, name)
	if err != nil {
		return Group{}, err
	}
	if err := txStore.addMember(ctx, g.ID, ownerID, RoleOwner); err != nil {
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, fmt.Errorf("group: commit: %w", err)
	}

	g.Role = RoleOwner
	return g, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Group, error) {
	return s.store.listForUser(ctx, userID)
}

// Get returns the group only if the requester belongs to it.
func (s *Service) Get(ctx context.Context, userID, groupID uuid.UUID) (Group, error) {
	return s.store.findForMember(ctx, groupID, userID)
}

// Members lists the group's membership, for members only.
func (s *Service) Members(ctx context.Context, userID, groupID uuid.UUID) ([]Member, error) {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		return nil, err
	}
	return s.store.listMembers(ctx, groupID)
}

// CreateInvite issues a shareable code. Any member may invite: these are small
// private groups where requiring the owner would only add friction.
func (s *Service) CreateInvite(ctx context.Context, userID, groupID uuid.UUID) (Invite, error) {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		return Invite{}, err
	}

	code, err := newInviteCode()
	if err != nil {
		return Invite{}, err
	}
	return s.store.createInvite(ctx, groupID, userID, code, s.now().Add(InviteLifetime))
}

// ListInvites shows the group's usable invites. Any member may see them,
// matching who may create them; only the owner may revoke.
func (s *Service) ListInvites(ctx context.Context, userID, groupID uuid.UUID) ([]Invite, error) {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		return nil, err
	}
	return s.store.listActiveInvites(ctx, groupID, s.now())
}

// RevokeInvite kills a leaked link. Owner only, so one member cannot undo
// another's invite.
func (s *Service) RevokeInvite(ctx context.Context, userID, groupID, inviteID uuid.UUID) error {
	g, err := s.store.findForMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if g.OwnerID != userID {
		return ErrNotOwner
	}
	return s.store.revokeInvite(ctx, groupID, inviteID, s.now())
}

// Join adds the user to the invite's group. Joining twice is not an error: the
// natural thing to do with a link that seems not to have worked is click it
// again.
func (s *Service) Join(ctx context.Context, userID uuid.UUID, code string) (Group, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Group{}, ErrInviteInvalid
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, fmt.Errorf("group: begin: %w", err)
	}
	defer tx.Rollback()

	txStore := &store{db: tx}
	invite, err := txStore.findUsableInvite(ctx, code, s.now())
	if err != nil {
		return Group{}, err
	}
	if err := txStore.addMember(ctx, invite.GroupID, userID, RoleMember); err != nil {
		return Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return Group{}, fmt.Errorf("group: commit: %w", err)
	}

	return s.store.findForMember(ctx, invite.GroupID, userID)
}

// IsMember reports whether a user currently belongs to a group. Other domains
// use it to scope what happens inside one without reading this package's
// tables, so leaving a group takes effect everywhere at once.
func (s *Service) IsMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		if errors.Is(err, ErrNotMember) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SharesGroup reports whether two users are currently in a group together.
//
// It exists so other domains can scope what one user may see of another
// without reading this package's tables: membership stays defined in one
// place, and leaving a group takes effect everywhere at once.
func (s *Service) SharesGroup(ctx context.Context, a, b uuid.UUID) (bool, error) {
	if a == b {
		return true, nil
	}
	return s.store.sharesGroup(ctx, a, b)
}

// Leave removes the requester from the group. The owner is refused, which is
// what keeps a group from becoming ownerless in a version with no ownership
// transfer.
func (s *Service) Leave(ctx context.Context, userID, groupID uuid.UUID) error {
	g, err := s.store.findForMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if g.OwnerID == userID {
		return ErrOwnerCannotLeave
	}
	return s.store.removeMember(ctx, groupID, userID)
}

// newInviteCode returns a code that is unguessable but still readable enough
// to be pasted into a chat message. Base32 avoids case-sensitivity surprises.
func newInviteCode() (string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("group: generate invite code: %w", err)
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}
