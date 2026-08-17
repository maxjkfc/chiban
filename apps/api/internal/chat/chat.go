// Package chat owns group conversation: messages, their history and the
// realtime fan-out to connected members.
//
// Two rules shape everything here. A message is persisted before it is
// broadcast, so nothing appears on screen that does not exist. And sending
// carries a client-generated ID, so a retry after a dropped response resolves
// to the message that already exists instead of posting it twice.
package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrNotMember is returned for groups the user cannot see, matching the
	// group domain's answer so chat never reveals more than membership does.
	ErrNotMember = errors.New("chat: not a member")
)

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("chat: invalid %s: %s", e.Field, e.Message)
}

const (
	// TypeText is the only kind V0.1 can send at this point; meal, image, gif
	// and sticker messages arrive with the slices that produce them.
	TypeText = "text"

	maxContentLength = 2000
	// DefaultPageSize matches what a phone screen can show without the first
	// scroll feeling empty.
	DefaultPageSize = 30
	MaxPageSize     = 100
)

type Message struct {
	ID              uuid.UUID
	GroupID         uuid.UUID
	UserID          uuid.UUID
	Type            string
	Content         string
	ClientMessageID uuid.UUID
	CreatedAt       time.Time
}

// Cursor is where this message sits in its group's history.
func (m Message) Cursor() Cursor {
	return Cursor{CreatedAt: m.CreatedAt, ID: m.ID}
}

// Page is one screen of history, oldest first, with a cursor for the screen
// before it.
type Page struct {
	Messages []Message
	// Before fetches the previous page; empty once history runs out.
	Before string
}

// SendInput is one outgoing message.
type SendInput struct {
	Content         string
	ClientMessageID uuid.UUID
}

// Membership answers whether a user belongs to a group. The group domain
// implements it; keeping it an interface here means chat never reads
// membership tables it does not own.
type Membership interface {
	IsMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error)
}

type Service struct {
	store   *store
	members Membership
	hub     *Hub
}

func NewService(db DBTX, members Membership, hub *Hub) *Service {
	return &Service{store: &store{db: db}, members: members, hub: hub}
}

// Send stores a message and then hands the stored row to everyone connected.
//
// The order is the point: the broadcast carries what the database returned,
// never what the client sent, so a message on screen is one that survived the
// write. A repeated client_message_id resolves to the existing row instead of
// inserting a second one, which is what makes retrying safe.
func (s *Service) Send(ctx context.Context, userID, groupID uuid.UUID, in SendInput) (Message, error) {
	if err := s.requireMember(ctx, userID, groupID); err != nil {
		return Message{}, err
	}

	content := strings.TrimSpace(in.Content)
	if content == "" {
		return Message{}, InvalidInputError{Field: "content", Message: "is required"}
	}
	if len([]rune(content)) > maxContentLength {
		return Message{}, InvalidInputError{
			Field:   "content",
			Message: fmt.Sprintf("must be at most %d characters", maxContentLength),
		}
	}
	if in.ClientMessageID == uuid.Nil {
		return Message{}, InvalidInputError{
			Field:   "client_message_id",
			Message: "is required so a retry does not post twice",
		}
	}

	message, inserted, err := s.store.insertOrGet(ctx, groupID, userID, TypeText, content, in.ClientMessageID)
	if err != nil {
		return Message{}, err
	}
	if inserted {
		// Only a genuinely new message is announced; a retry must not make
		// everyone's screen show it twice.
		s.hub.Broadcast(groupID, message)
	}
	return message, nil
}

// History returns the page ending at before, or the newest page when before is
// empty. Messages come back oldest-first, which is the order they are read in.
func (s *Service) History(ctx context.Context, userID, groupID uuid.UUID, before string, limit int) (Page, error) {
	if err := s.requireMember(ctx, userID, groupID); err != nil {
		return Page{}, err
	}

	switch {
	case limit <= 0:
		limit = DefaultPageSize
	case limit > MaxPageSize:
		limit = MaxPageSize
	}

	var cursor Cursor
	if before != "" {
		parsed, err := ParseCursor(before)
		if err != nil {
			return Page{}, err
		}
		cursor = parsed
	}

	// One extra row answers "is there more" without a second query.
	messages, err := s.store.listBefore(ctx, groupID, cursor, limit+1)
	if err != nil {
		return Page{}, err
	}

	page := Page{}
	if len(messages) > limit {
		messages = messages[:limit]
		// messages is newest-first here, so the last one is the oldest shown.
		page.Before = messages[len(messages)-1].Cursor().String()
	}

	// Flip to oldest-first for display.
	page.Messages = make([]Message, 0, len(messages))
	for i := len(messages) - 1; i >= 0; i-- {
		page.Messages = append(page.Messages, messages[i])
	}
	return page, nil
}

// Subscribe attaches a connection to a group's fan-out, after checking that
// the user is allowed to be there. The realtime path is authorised exactly
// like the REST one: a non-member must not be able to listen in.
func (s *Service) Subscribe(ctx context.Context, userID, groupID uuid.UUID) (*Subscription, error) {
	if err := s.requireMember(ctx, userID, groupID); err != nil {
		return nil, err
	}
	return s.hub.Subscribe(groupID), nil
}

func (s *Service) requireMember(ctx context.Context, userID, groupID uuid.UUID) error {
	member, err := s.members.IsMember(ctx, userID, groupID)
	if err != nil {
		return err
	}
	if !member {
		return ErrNotMember
	}
	return nil
}

// DBTX is the slice of database/sql this package needs.
type DBTX interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
