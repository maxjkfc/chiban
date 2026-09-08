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
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/storage"
)

var (
	// ErrNotMember is returned for groups the user cannot see, matching the
	// group domain's answer so chat never reveals more than membership does.
	ErrNotMember = errors.New("chat: not a member")
	// ErrNotFound is returned for a message that does not exist.
	ErrNotFound = errors.New("chat: message not found")
	// ErrNotAuthor covers deleting someone else's message: you can see it, you
	// just do not get to remove it.
	ErrNotAuthor = errors.New("chat: not the author")
)

// DefaultReactions is the default set offered to users.
var DefaultReactions = []string{"❤️", "😂", "🔥", "👏", "👀"}

// Reactions is retained for backwards compatibility with availableReactionsHandler.
var Reactions = DefaultReactions

func isValidReaction(reactionType string) bool {
	trimmed := strings.TrimSpace(reactionType)
	if trimmed == "" {
		return false
	}
	// Database CHECK constraint enforces length between 1 and 16 bytes.
	// In UTF-8, any standard single Emoji (including skin-tone modifiers or ZWJ sequences)
	// fits comfortably within 16 bytes.
	if len(trimmed) > 16 {
		return false
	}
	return utf8.ValidString(trimmed)
}

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("chat: invalid %s: %s", e.Field, e.Message)
}

const (
	// TypeText is the only kind a person types; the rest are produced by the
	// features that create them.
	TypeText = "text"
	// TypeMeal is a shared meal's card. It carries a reference and nothing
	// else — never a copy of the meal's description or photos.
	TypeMeal = "meal"
	// TypeImage and TypeGIF are uploads posted into a conversation. Like a
	// meal card they carry only a reference; the bytes are read through the
	// media endpoint, which decides who may see them.
	TypeImage = "image"
	TypeGIF   = "gif"
	// TypeSticker is one of the sender's own stickers. It carries only the
	// sticker id; the bytes come from the sticker endpoint, and an image
	// sticker and a GIF sticker are the same kind of message because what a
	// reader does with either is identical.
	TypeSticker = "sticker"

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
	// Deleted marks a tombstone. The row stays where it was so replies to it
	// keep their context; only the content is gone.
	Deleted bool
	// ChatMediaID is the upload an image or GIF message shows.
	ChatMediaID *uuid.UUID
	// StickerID is the sticker a sticker message shows.
	StickerID *uuid.UUID
	// MealRecordID is what a meal card points at. The card's contents are
	// fetched from the meal itself, so they cannot drift from it or outlive it.
	MealRecordID *uuid.UUID
	// ReplyTo is the message this one answers, when it answers one. It is
	// joined at read time rather than copied at write time, so deleting the
	// parent is reflected everywhere at once.
	ReplyTo *ReplyPreview
	// Reactions summarises who reacted with what, from the perspective of the
	// reader who asked. Never filled on a broadcast: Mine is per-reader and a
	// broadcast goes to everyone.
	Reactions []ReactionSummary
}

// ReplyPreview is as much of the parent as a quote needs.
type ReplyPreview struct {
	ID     uuid.UUID
	UserID uuid.UUID
	// Type is what the parent was. Only a text message has content to quote;
	// every other kind stores none, so without this a quote of one is an empty
	// box and the reply reads as an answer to nothing.
	Type    string
	Content string
	Deleted bool
}

// ReactionSummary is one emoji's tally on a message.
type ReactionSummary struct {
	Type  string
	Count int
	// Mine is whether the reader who asked is one of the Count.
	Mine bool
}

// ReactionChange is one person adding or removing one reaction.
type ReactionChange struct {
	MessageID uuid.UUID
	UserID    uuid.UUID
	Type      string
	Added     bool
}

// Event kinds delivered over the socket.
const (
	EventMessage  = "message"
	EventReaction = "reaction"
	EventDeleted  = "deleted"
)

// Event is one thing that happened in a group.
//
// Which field carries the payload depends on Kind; the others are zero. A
// deletion sends only an id rather than the tombstoned message, because a
// message carries per-reader reaction state that a single broadcast cannot.
type Event struct {
	Kind       string
	GroupIDVal uuid.UUID
	Message    Message
	Reaction   ReactionChange
	MessageID  uuid.UUID
}

func (e Event) GroupID() uuid.UUID {
	if e.GroupIDVal != uuid.Nil {
		return e.GroupIDVal
	}
	if e.Message.GroupID != uuid.Nil {
		return e.Message.GroupID
	}
	return uuid.Nil
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
	// ReplyToMessageID makes this message an answer to another one in the same
	// group. A reply is an ordinary message; V0.1 has no comment domain.
	ReplyToMessageID *uuid.UUID
	// ChatMediaID posts an already-uploaded image or GIF instead of text. The
	// upload happens first and separately, so a failed send never leaves a
	// half-written message pointing at nothing.
	ChatMediaID *uuid.UUID
	// StickerID posts one of the sender's own stickers instead of text.
	StickerID *uuid.UUID
}

// Membership answers whether a user belongs to a group. The group domain
// implements it; keeping it an interface here means chat never reads
// membership tables it does not own.
type Membership interface {
	IsMember(ctx context.Context, userID, groupID uuid.UUID) (bool, error)
}

// Stickers answers whether a sticker is the sender's to send. The sticker
// domain implements it; keeping it an interface here means chat never reads
// the sticker table it does not own.
type Stickers interface {
	BelongsTo(ctx context.Context, stickerID, userID uuid.UUID) (bool, error)
}
type PushNotifier interface {
	NotifyMessage(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, messageType string)
}

// ReadCursors advances a member's own read cursor. The group domain owns the
// cursor and implements this; chat calls it after a send succeeds so that
// posting a message never leaves the sender's own group looking unread to
// them.
type ReadCursors interface {
	AdvanceOwnCursor(ctx context.Context, userID, groupID, messageID uuid.UUID, messageCreatedAt time.Time) error
}

type Service struct {
	store       *store
	members     Membership
	hub         *Hub
	objects     storage.ObjectStorage
	stickers    Stickers
	push        PushNotifier
	readCursors ReadCursors
}

func NewService(
	db DBTX,
	members Membership,
	hub *Hub,
	objects storage.ObjectStorage,
	stickers Stickers,
) *Service {
	return &Service{
		store:    &store{db: db},
		members:  members,
		hub:      hub,
		objects:  objects,
		stickers: stickers,
	}
}
func (s *Service) SetPushNotifier(p PushNotifier) {
	s.push = p
}

// SetReadCursors wires in the group domain's cursor advance, after both
// services exist.
func (s *Service) SetReadCursors(r ReadCursors) {
	s.readCursors = r
}

// markSenderRead advances the sender's own cursor to the message they just
// posted, best-effort: a failure here must not undo a message that already
// sent successfully, it would just leave the sender's own group looking
// unread to them until their client marks it read explicitly.
func (s *Service) markSenderRead(ctx context.Context, userID, groupID, messageID uuid.UUID, createdAt time.Time) {
	if s.readCursors == nil {
		return
	}
	_ = s.readCursors.AdvanceOwnCursor(ctx, userID, groupID, messageID, createdAt)
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

	messageType := TypeText
	content := strings.TrimSpace(in.Content)

	if in.ChatMediaID != nil && in.StickerID != nil {
		// One message shows one thing. Allowing both would make the message
		// type a guess about which of them the reader is meant to see.
		return Message{}, InvalidInputError{
			Field:   "sticker_id",
			Message: "cannot be sent together with an upload",
		}
	}

	switch {
	case in.StickerID != nil:
		// The sticker has to be the sender's own and still in their library.
		// Sending someone else's would let anyone paste any sticker id.
		mine, err := s.stickers.BelongsTo(ctx, *in.StickerID, userID)
		if err != nil {
			return Message{}, fmt.Errorf("chat: check sticker: %w", err)
		}
		if !mine {
			return Message{}, InvalidInputError{
				Field:   "sticker_id",
				Message: "is not one of your stickers",
			}
		}
		messageType = TypeSticker
		content = ""
	case in.ChatMediaID != nil:
		// The upload has to be the sender's own. Posting someone else's would
		// widen who can read it to a group its uploader never chose.
		mediaType, err := s.store.findOwnedMedia(ctx, *in.ChatMediaID, userID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return Message{}, InvalidInputError{
					Field:   "chat_media_id",
					Message: "is not one of your uploads",
				}
			}
			return Message{}, err
		}
		// The kind comes from what was actually uploaded, never from the
		// client: a GIF posted as an image would be re-encoded on the way out.
		messageType = mediaType
		content = ""
	default:
		if content == "" {
			return Message{}, InvalidInputError{Field: "content", Message: "is required"}
		}
		if len([]rune(content)) > maxContentLength {
			return Message{}, InvalidInputError{
				Field:   "content",
				Message: fmt.Sprintf("must be at most %d characters", maxContentLength),
			}
		}
	}
	if in.ClientMessageID == uuid.Nil {
		return Message{}, InvalidInputError{
			Field:   "client_message_id",
			Message: "is required so a retry does not post twice",
		}
	}

	if in.ReplyToMessageID != nil {
		// A reply must point at something in the same conversation, or the
		// quote would show a stranger's message from a group you cannot read.
		parent, err := s.store.get(ctx, *in.ReplyToMessageID)
		switch {
		case errors.Is(err, ErrNotFound), err == nil && parent.GroupID != groupID:
			return Message{}, InvalidInputError{
				Field:   "reply_to_message_id",
				Message: "is not a message in this group",
			}
		case err != nil:
			return Message{}, err
		}
	}

	id, inserted, err := s.store.insertOrGet(ctx, groupID, userID, messageType, content,
		in.ClientMessageID, in.ReplyToMessageID, in.ChatMediaID, in.StickerID)
	if err != nil {
		return Message{}, err
	}

	message, err := s.store.get(ctx, id)
	if err != nil {
		return Message{}, err
	}
	if message.GroupID != groupID {
		// The id is unique per sender, not per sender and group, so a client
		// that reuses one across groups would otherwise be handed a message
		// belonging to a conversation it did not ask about. Say so instead.
		return Message{}, InvalidInputError{
			Field:   "client_message_id",
			Message: "was already used for a message in another group",
		}
	}

	if inserted {
		// Only a genuinely new message is announced; a retry must not make
		// everyone's screen show it twice.
		s.hub.Broadcast(groupID, Event{Kind: EventMessage, Message: message})
		s.markSenderRead(ctx, userID, groupID, message.ID, message.CreatedAt)
		if s.push != nil {
			s.push.NotifyMessage(ctx, groupID, userID, message.Type)
		}
	}
	return message, nil
}

// Delete tombstones the caller's own message.
//
// The row stays: replies to it keep their place in the conversation and still
// show what they were answering, which is the whole point of not cascading.
func (s *Service) Delete(ctx context.Context, userID, messageID uuid.UUID) error {
	message, err := s.store.get(ctx, messageID)
	if err != nil {
		return err
	}
	if err := s.requireMember(ctx, userID, message.GroupID); err != nil {
		return err
	}
	if message.UserID != userID {
		return ErrNotAuthor
	}

	deleted, err := s.store.softDelete(ctx, messageID)
	if err != nil {
		return err
	}
	if deleted {
		s.hub.Broadcast(message.GroupID, Event{Kind: EventDeleted, GroupIDVal: message.GroupID, MessageID: messageID})
	}
	return nil
}

// React adds one of the caller's reactions to a message.
//
// Repeating it is deliberately not an error: a double tap on a phone must
// leave one reaction and one broadcast, not two of either.
func (s *Service) React(ctx context.Context, userID, messageID uuid.UUID, reactionType string) error {
	return s.changeReaction(ctx, userID, messageID, reactionType, true)
}

// Unreact takes back one of the caller's own reactions. There is no way to
// remove anyone else's: the delete is scoped by user_id in the statement.
func (s *Service) Unreact(ctx context.Context, userID, messageID uuid.UUID, reactionType string) error {
	return s.changeReaction(ctx, userID, messageID, reactionType, false)
}

func (s *Service) changeReaction(
	ctx context.Context,
	userID, messageID uuid.UUID,
	reactionType string,
	add bool,
) error {
	if !isValidReaction(reactionType) {
		return InvalidInputError{
			Field:   "reaction_type",
			Message: "must be a valid emoji up to 16 bytes",
		}
	}

	message, err := s.store.get(ctx, messageID)
	if err != nil {
		return err
	}
	// Reacting is writing into a group, so it is gated on membership exactly
	// like sending is.
	if err := s.requireMember(ctx, userID, message.GroupID); err != nil {
		return err
	}
	if add && message.Deleted {
		return InvalidInputError{Field: "message_id", Message: "has been deleted"}
	}

	var changed bool
	if add {
		changed, err = s.store.addReaction(ctx, messageID, userID, reactionType)
	} else {
		changed, err = s.store.removeReaction(ctx, messageID, userID, reactionType)
	}
	if err != nil {
		return err
	}

	if add && !changed {
		// Either this reaction already existed, or the message was deleted
		// between reading it and writing. The statement refuses both the same
		// way, so ask which it was rather than answering "done" to a caller
		// whose reaction never landed.
		switch fresh, err := s.store.get(ctx, messageID); {
		case err != nil:
			return err
		case fresh.Deleted:
			return InvalidInputError{Field: "message_id", Message: "has been deleted"}
		}
	}

	if changed {
		s.hub.Broadcast(message.GroupID, Event{
			Kind:       EventReaction,
			GroupIDVal: message.GroupID,
			Reaction: ReactionChange{
				MessageID: messageID,
				UserID:    userID,
				Type:      reactionType,
				Added:     add,
			},
		})
	}
	return nil
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

	if err := s.attachReactions(ctx, userID, page.Messages); err != nil {
		return Page{}, err
	}
	return page, nil
}

// attachReactions fills in a page's tallies in one query rather than one per
// message.
func (s *Service) attachReactions(ctx context.Context, readerID uuid.UUID, messages []Message) error {
	ids := make([]uuid.UUID, 0, len(messages))
	for _, m := range messages {
		ids = append(ids, m.ID)
	}

	byMessage, err := s.store.reactionsFor(ctx, readerID, ids)
	if err != nil {
		return err
	}
	for i := range messages {
		messages[i].Reactions = byMessage[messages[i].ID]
	}
	return nil
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
func (s *Service) SubscribeUser(ctx context.Context, userID uuid.UUID) *UserSubscription {
	return s.hub.SubscribeUser(userID)
}

// MayReceive reports whether this user is still entitled to a group's
// messages.
//
// A socket outlives the check that opened it: someone can leave the group with
// the connection still open, and leaving has to take effect immediately rather
// than whenever they happen to reconnect. Asking again before each delivery
// keeps that true for every way membership can end, including ones added
// later.
//
// ponytail: one indexed lookup per delivered message per connection, which is
// nothing at V0.1's group sizes. Cache it against an invalidation signal if a
// profile ever says otherwise.
func (s *Service) MayReceive(ctx context.Context, userID, groupID uuid.UUID) (bool, error) {
	return s.members.IsMember(ctx, userID, groupID)
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

// AnnounceMeal posts a shared meal's card into a group.
//
// The message carries the meal's id and nothing else: no description, no photo
// paths. Whoever reads it fetches the meal through the meal API, which decides
// there and then whether they may see it — so a card cannot outlive the share
// that justified it.
//
// Membership is not re-checked here. The caller is the meal domain, which has
// already established that this user may share into this group; a second check
// would be a different question asked of the same fact.
func (s *Service) AnnounceMeal(ctx context.Context, userID, groupID, mealID uuid.UUID) error {
	id, inserted, err := s.store.insertMeal(ctx, groupID, userID, mealID)
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	message, err := s.store.get(ctx, id)
	if err != nil {
		return err
	}
	s.hub.Broadcast(groupID, Event{Kind: EventMessage, Message: message})
	s.markSenderRead(ctx, userID, groupID, message.ID, message.CreatedAt)
	return nil
}

// Locate reports where a message sits in a group's history, for the group
// domain's read-cursor validation.
//
// It matches on group_id explicitly rather than trusting the id alone: a
// message id from a different group must not validate a cursor claiming to
// be for this one. Tombstones resolve normally — marking read up to a
// deleted message is a real position in the conversation.
func (s *Service) Locate(ctx context.Context, groupID, messageID uuid.UUID) (time.Time, bool, error) {
	return s.store.locate(ctx, groupID, messageID)
}

// UnreadCounts tallies, for each of a batch of groups, how many messages —
// including tombstones — sit after that group's read cursor.
//
// groupIDs, cursorMessageIDs and cursorCreatedAts are parallel slices: index i
// of each describes one group. A zero-value cursor (uuid.Nil) means the
// reader has never marked that group read, so every message in it is unread.
func (s *Service) UnreadCounts(
	ctx context.Context,
	groupIDs, cursorMessageIDs []uuid.UUID,
	cursorCreatedAts []time.Time,
) (map[uuid.UUID]int, error) {
	return s.store.unreadCounts(ctx, groupIDs, cursorMessageIDs, cursorCreatedAts)
}
