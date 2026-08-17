package chat

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// errNoRows is the package-internal signal that a statement returned nothing.
var errNoRows = sql.ErrNoRows

type store struct {
	db DBTX
}

// insertOrGet writes the message, or returns the one this client already sent.
//
// The retry case is not an error: a phone that lost the response and tried
// again must end up with exactly one message and be told which one it is.
// ON CONFLICT makes that a single statement, so two racing retries cannot both
// decide they are the first.
func (s *store) insertOrGet(
	ctx context.Context,
	groupID, userID uuid.UUID,
	messageType, content string,
	clientMessageID uuid.UUID,
) (Message, bool, error) {
	m := Message{
		GroupID:         groupID,
		UserID:          userID,
		Type:            messageType,
		ClientMessageID: clientMessageID,
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (group_id, user_id, message_type, content, client_message_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, client_message_id) DO NOTHING
		RETURNING id, coalesce(content, ''), created_at
	`, groupID, userID, messageType, content, clientMessageID).
		Scan(&m.ID, &m.Content, &m.CreatedAt)
	if err == nil {
		return m, true, nil
	}
	if !errors.Is(err, errNoRows) {
		if isForeignKeyViolation(err) {
			// The group vanished between the membership check and the write.
			return Message{}, false, ErrNotMember
		}
		return Message{}, false, fmt.Errorf("chat: insert message: %w", err)
	}

	// DO NOTHING returned no row, so this client_message_id is already used.
	existing, err := s.findByClientMessageID(ctx, userID, clientMessageID)
	if err != nil {
		return Message{}, false, err
	}
	if existing.GroupID != groupID {
		// The id is unique per sender, not per sender and group, so a client
		// that reuses one across groups would otherwise be handed a message
		// belonging to a conversation it did not ask about. Say so instead.
		return Message{}, false, InvalidInputError{
			Field:   "client_message_id",
			Message: "was already used for a message in another group",
		}
	}
	return existing, false, nil
}

func (s *store) findByClientMessageID(ctx context.Context, userID, clientMessageID uuid.UUID) (Message, error) {
	var m Message
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, user_id, message_type, coalesce(content, ''), client_message_id, created_at
		FROM chat_messages
		WHERE user_id = $1 AND client_message_id = $2
	`, userID, clientMessageID).Scan(
		&m.ID, &m.GroupID, &m.UserID, &m.Type, &m.Content, &m.ClientMessageID, &m.CreatedAt)
	if err != nil {
		return Message{}, fmt.Errorf("chat: find message by client id: %w", err)
	}
	return m, nil
}

// listBefore returns messages newest-first, starting just before the cursor.
//
// The comparison is on the pair (created_at, id) rather than the timestamp
// alone, so messages sharing a timestamp are neither repeated on the next page
// nor skipped between pages.
func (s *store) listBefore(ctx context.Context, groupID uuid.UUID, cursor Cursor, limit int) ([]Message, error) {
	query := `
		SELECT id, group_id, user_id, message_type, coalesce(content, ''), client_message_id, created_at
		FROM chat_messages
		WHERE group_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
		LIMIT $2`
	args := []any{groupID, limit}

	if !cursor.isZero() {
		query = `
			SELECT id, group_id, user_id, message_type, coalesce(content, ''), client_message_id, created_at
			FROM chat_messages
			WHERE group_id = $1 AND deleted_at IS NULL AND (created_at, id) < ($3, $4)
			ORDER BY created_at DESC, id DESC
			LIMIT $2`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("chat: list messages: %w", err)
	}
	defer rows.Close()

	messages := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Type, &m.Content,
			&m.ClientMessageID, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("chat: scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
