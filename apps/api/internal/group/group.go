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
	// ErrMessageNotInGroup covers marking read up to a message that either
	// does not exist or belongs to a different group. The client cannot
	// backdate someone else's cursor by guessing an id from another group.
	ErrMessageNotInGroup = errors.New("group: message is not in this group")
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
	// ReadCursor is the caller's own position in this group's chat. Never
	// filled for anyone but the requester: another member's read state is
	// not this endpoint's business.
	ReadCursor ReadCursor
	// UnreadCount is how many messages, including tombstones, sit after
	// ReadCursor. Populated by ListForUser; zero value elsewhere.
	UnreadCount int
	// Pinned is the caller's own choice to keep this group at the top of
	// their list. Per-user, not a group property: never filled for anyone
	// but the requester.
	Pinned bool
}

type Member struct {
	UserID   uuid.UUID
	Role     string
	JoinedAt time.Time
}

// ReadCursor is how far a member has read a group's chat.
//
// It names a message rather than a moment: the pair (created_at, id) matches
// the chat history cursor exactly, so unread counting agrees with pagination
// on what "before" and "after" mean and never drifts with the clock.
type ReadCursor struct {
	MessageID uuid.UUID
	CreatedAt time.Time
}

// IsZero reports a member who has never marked anything read, so every
// message in the group is still unread.
func (c ReadCursor) IsZero() bool { return c.MessageID == uuid.Nil }

type Invite struct {
	ID        uuid.UUID
	GroupID   uuid.UUID
	Code      string
	ExpiresAt time.Time
}

// MessageLocator answers where a message sits in a specific group's history,
// so a read cursor can be validated against the group it claims to belong to
// without this package reading chat's own tables.
//
// A tombstoned message still resolves: marking read up to a deleted message
// is a real position in the conversation, not a no-op, so soft-deletion must
// not make Locate report it missing.
type MessageLocator interface {
	Locate(ctx context.Context, groupID, messageID uuid.UUID) (createdAt time.Time, ok bool, err error)
}

// UnreadCounter tallies, for a batch of groups, how many messages — including
// tombstones — sit after each group's read cursor.
//
// The three slices are parallel and share groupIDs' length. A uuid.Nil cursor
// message id means the reader has never marked that group read, so every
// message in it counts.
type UnreadCounter interface {
	UnreadCounts(
		ctx context.Context,
		groupIDs, cursorMessageIDs []uuid.UUID,
		cursorCreatedAts []time.Time,
	) (map[uuid.UUID]int, error)
}

// ChatReader is what the chat domain gives back so unread state can be
// computed here without group reading chat_messages directly. Chat
// implements it; wiring happens after both services exist, the same way
// push notification wiring does.
type ChatReader interface {
	MessageLocator
	UnreadCounter
}

type Service struct {
	db    *sql.DB
	store *store
	now   func() time.Time
	chat  ChatReader
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db, store: &store{db: db}, now: time.Now}
}

// SetChatReader wires in the chat domain's answers about message position and
// unread counts. Called once at startup, after both services exist.
func (s *Service) SetChatReader(c ChatReader) {
	s.chat = c
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

// ListForUser lists the groups a user belongs to, along with how many
// messages are unread in each.
//
// Unread counting is best-effort against the chat domain: if it fails, the
// group list itself must still render, just without counts, rather than
// taking the whole nav down over a feature that is purely decorative.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID) ([]Group, error) {
	groups, err := s.store.listForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.attachUnreadCounts(ctx, groups), nil
}

// Get returns the group only if the requester belongs to it, with the
// caller's own unread count for it.
func (s *Service) Get(ctx context.Context, userID, groupID uuid.UUID) (Group, error) {
	g, err := s.store.findForMember(ctx, groupID, userID)
	if err != nil {
		return Group{}, err
	}
	groups := s.attachUnreadCounts(ctx, []Group{g})
	return groups[0], nil
}

// attachUnreadCounts fills in UnreadCount for each group in one round trip to
// chat, best-effort: a failure here must not take down whatever endpoint is
// listing or reading groups, since the count is purely decorative.
func (s *Service) attachUnreadCounts(ctx context.Context, groups []Group) []Group {
	if s.chat == nil || len(groups) == 0 {
		return groups
	}

	groupIDs := make([]uuid.UUID, len(groups))
	cursorIDs := make([]uuid.UUID, len(groups))
	cursorAts := make([]time.Time, len(groups))
	for i, g := range groups {
		groupIDs[i] = g.ID
		cursorIDs[i] = g.ReadCursor.MessageID
		cursorAts[i] = g.ReadCursor.CreatedAt
	}

	counts, err := s.chat.UnreadCounts(ctx, groupIDs, cursorIDs, cursorAts)
	if err != nil {
		return groups
	}
	for i := range groups {
		groups[i].UnreadCount = counts[groups[i].ID]
	}
	return groups
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

// MarkRead advances the caller's own read cursor for a group to the given
// message.
//
// The message must belong to this group: a client-supplied id from another
// group must not be accepted, or a member could forge having read messages
// they were never shown. Moving the cursor backward is refused too — a
// stale client replaying an old "mark read" after a newer one must not
// resurrect messages the user already dismissed.
func (s *Service) MarkRead(ctx context.Context, userID, groupID, messageID uuid.UUID) error {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		return err
	}
	if s.chat == nil {
		return fmt.Errorf("group: mark read: chat reader is not wired")
	}

	createdAt, ok, err := s.chat.Locate(ctx, groupID, messageID)
	if err != nil {
		return fmt.Errorf("group: locate message: %w", err)
	}
	if !ok {
		return ErrMessageNotInGroup
	}

	return s.store.advanceReadCursor(ctx, groupID, userID, messageID, createdAt)
}

// AdvanceOwnCursor moves a member's read cursor forward without the
// membership or Locate round trip MarkRead does.
//
// It exists for chat to call right after a member's own message is stored:
// the caller (chat) already knows the message belongs to this group and that
// the sender is a member, because chat just checked both to accept the send.
// Re-deriving that here would be the same two facts asked a second time.
func (s *Service) AdvanceOwnCursor(ctx context.Context, userID, groupID, messageID uuid.UUID, messageCreatedAt time.Time) error {
	return s.store.advanceReadCursor(ctx, groupID, userID, messageID, messageCreatedAt)
}

// Pin keeps a group at the top of the caller's own list, on every device.
//
// Only a member may pin a group they belong to: findForMember doubles as the
// authorization check and turns a non-member's attempt into ErrNotMember,
// the same answer a non-member gets from every other group read. Pinning an
// already-pinned group is a no-op, not an error.
func (s *Service) Pin(ctx context.Context, userID, groupID uuid.UUID) error {
	if _, err := s.store.findForMember(ctx, groupID, userID); err != nil {
		return err
	}
	return s.store.pinGroup(ctx, userID, groupID, s.now())
}

// Unpin removes the caller's own pin. Unpinning a group that was never
// pinned, or one the caller has since left, is a no-op: the end state the
// caller wants — this group not pinned — already holds.
func (s *Service) Unpin(ctx context.Context, userID, groupID uuid.UUID) error {
	return s.store.unpinGroup(ctx, userID, groupID)
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

// GroupIDsFor lists the groups a user belongs to. Meal reads it to decide
// which shared meals that user may see, without touching membership tables it
// does not own.
func (s *Service) GroupIDsFor(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	return s.store.groupIDsFor(ctx, userID)
}
