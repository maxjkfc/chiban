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

// messageQuery is the one definition of how a message is read.
//
// The parent is joined rather than copied at write time, so a message that is
// deleted reads as deleted everywhere it is quoted, without touching the rows
// that quote it.
const messageQuery = `
	SELECT m.id, m.group_id, m.user_id, m.message_type, coalesce(m.content, ''),
	       m.client_message_id, m.created_at, m.deleted_at, m.meal_record_id,
	       parent.id, parent.user_id, coalesce(parent.content, ''), parent.deleted_at
	FROM chat_messages m
	LEFT JOIN chat_messages parent ON parent.id = m.reply_to_message_id`

// scanner is whatever a row can be read from, so one scan serves both the
// single-row and the paged query.
type scanner interface {
	Scan(dest ...any) error
}

func scanMessage(row scanner) (Message, error) {
	var (
		m          Message
		deletedAt  sql.NullTime
		mealID     uuid.NullUUID
		parentID   uuid.NullUUID
		parentUser uuid.NullUUID
		parentText string
		parentGone sql.NullTime
	)

	if err := row.Scan(&m.ID, &m.GroupID, &m.UserID, &m.Type, &m.Content,
		&m.ClientMessageID, &m.CreatedAt, &deletedAt, &mealID,
		&parentID, &parentUser, &parentText, &parentGone); err != nil {
		return Message{}, err
	}

	if deletedAt.Valid {
		// A tombstone keeps its place and loses its content. Blanking here
		// rather than at the edge means no caller can hand it out by accident.
		m.Deleted = true
		m.Content = ""
	}
	if mealID.Valid {
		m.MealRecordID = &mealID.UUID
	}
	if parentID.Valid {
		preview := ReplyPreview{
			ID:      parentID.UUID,
			UserID:  parentUser.UUID,
			Content: parentText,
			Deleted: parentGone.Valid,
		}
		if preview.Deleted {
			preview.Content = ""
		}
		m.ReplyTo = &preview
	}
	return m, nil
}

// insertOrGet writes the message, or finds the one this client already sent.
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
	replyTo *uuid.UUID,
) (uuid.UUID, bool, error) {
	var id uuid.UUID

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chat_messages
			(group_id, user_id, message_type, content, client_message_id, reply_to_message_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, client_message_id) DO NOTHING
		RETURNING id
	`, groupID, userID, messageType, content, clientMessageID, replyTo).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, errNoRows) {
		if isForeignKeyViolation(err) {
			// The group vanished between the membership check and the write.
			return uuid.UUID{}, false, ErrNotMember
		}
		return uuid.UUID{}, false, fmt.Errorf("chat: insert message: %w", err)
	}

	// DO NOTHING returned no row, so this client_message_id is already used.
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM chat_messages WHERE user_id = $1 AND client_message_id = $2
	`, userID, clientMessageID).Scan(&id)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("chat: find message by client id: %w", err)
	}
	return id, false, nil
}

func (s *store) get(ctx context.Context, id uuid.UUID) (Message, error) {
	message, err := scanMessage(s.db.QueryRowContext(ctx, messageQuery+` WHERE m.id = $1`, id))
	if errors.Is(err, errNoRows) {
		return Message{}, ErrNotFound
	}
	if err != nil {
		return Message{}, fmt.Errorf("chat: get message: %w", err)
	}
	return message, nil
}

// listBefore returns messages newest-first, starting just before the cursor.
//
// The comparison is on the pair (created_at, id) rather than the timestamp
// alone, so messages sharing a timestamp are neither repeated on the next page
// nor skipped between pages. Deleted messages are included: they are
// tombstones, and dropping them would take their replies' context with them.
func (s *store) listBefore(ctx context.Context, groupID uuid.UUID, cursor Cursor, limit int) ([]Message, error) {
	query := messageQuery + `
		WHERE m.group_id = $1
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT $2`
	args := []any{groupID, limit}

	if !cursor.isZero() {
		query = messageQuery + `
			WHERE m.group_id = $1 AND (m.created_at, m.id) < ($3, $4)
			ORDER BY m.created_at DESC, m.id DESC
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
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("chat: scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// softDelete tombstones a message, reporting whether it was still live.
//
// The reactions go with it. They are answers to content that no longer exists,
// unlike replies, which are their own contribution to the conversation and so
// are deliberately left standing.
func (s *store) softDelete(ctx context.Context, id uuid.UUID) (bool, error) {
	var tombstoned int
	err := s.db.QueryRowContext(ctx, `
		WITH removed AS (
			UPDATE chat_messages
			SET deleted_at = now(), updated_at = now(), content = NULL
			WHERE id = $1 AND deleted_at IS NULL
			RETURNING id
		), cleared AS (
			DELETE FROM message_reactions WHERE message_id IN (SELECT id FROM removed)
		)
		SELECT count(*) FROM removed
	`, id).Scan(&tombstoned)
	if err != nil {
		return false, fmt.Errorf("chat: delete message: %w", err)
	}
	return tombstoned > 0, nil
}

// addReaction records one, reporting whether it was new. A repeated tap is not
// an error and must not count twice, which the primary key guarantees.
func (s *store) addReaction(ctx context.Context, messageID, userID uuid.UUID, reactionType string) (bool, error) {
	// The liveness test is part of the statement, not a check before it: a
	// message can be deleted between reading it and reacting to it, and a
	// reaction that outlives its message is exactly what the tombstone is
	// meant to prevent.
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO message_reactions (message_id, user_id, reaction_type)
		SELECT $1, $2, $3
		WHERE EXISTS (
			SELECT 1 FROM chat_messages WHERE id = $1 AND deleted_at IS NULL
		)
		ON CONFLICT DO NOTHING
	`, messageID, userID, reactionType)
	if err != nil {
		return false, fmt.Errorf("chat: add reaction: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// removeReaction takes back one of the caller's own, reporting whether there
// was one to take back. The user_id in the WHERE clause is the authorization:
// there is no statement here that can touch someone else's reaction.
func (s *store) removeReaction(ctx context.Context, messageID, userID uuid.UUID, reactionType string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM message_reactions
		WHERE message_id = $1 AND user_id = $2 AND reaction_type = $3
	`, messageID, userID, reactionType)
	if err != nil {
		return false, fmt.Errorf("chat: remove reaction: %w", err)
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// reactionsFor tallies a whole page's reactions in one query, from the asking
// reader's point of view.
func (s *store) reactionsFor(
	ctx context.Context,
	readerID uuid.UUID,
	messageIDs []uuid.UUID,
) (map[uuid.UUID][]ReactionSummary, error) {
	byMessage := map[uuid.UUID][]ReactionSummary{}
	if len(messageIDs) == 0 {
		return byMessage, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT message_id, reaction_type, count(*), bool_or(user_id = $2)
		FROM message_reactions
		WHERE message_id = ANY($1)
		GROUP BY message_id, reaction_type
		-- Oldest first, so a reaction does not jump position as others arrive.
		ORDER BY message_id, min(created_at), reaction_type
	`, messageIDs, readerID)
	if err != nil {
		return nil, fmt.Errorf("chat: list reactions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageID uuid.UUID
			summary   ReactionSummary
		)
		if err := rows.Scan(&messageID, &summary.Type, &summary.Count, &summary.Mine); err != nil {
			return nil, fmt.Errorf("chat: scan reaction: %w", err)
		}
		byMessage[messageID] = append(byMessage[messageID], summary)
	}
	return byMessage, rows.Err()
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

// insertMeal posts a meal card, or finds the one already posted.
//
// One card per meal per group: sharing again after revoking re-opens access
// without starting a second conversation about the same plate of food.
func (s *store) insertMeal(ctx context.Context, groupID, userID, mealID uuid.UUID) (uuid.UUID, bool, error) {
	var id uuid.UUID

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO chat_messages (group_id, user_id, message_type, client_message_id, meal_record_id)
		SELECT $1, $2, 'meal', gen_random_uuid(), $3
		WHERE NOT EXISTS (
			SELECT 1 FROM chat_messages
			WHERE group_id = $1 AND meal_record_id = $3 AND deleted_at IS NULL
		)
		RETURNING id
	`, groupID, userID, mealID).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, errNoRows) {
		return uuid.UUID{}, false, fmt.Errorf("chat: announce meal: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM chat_messages
		WHERE group_id = $1 AND meal_record_id = $2 AND deleted_at IS NULL
	`, groupID, mealID).Scan(&id)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("chat: find meal message: %w", err)
	}
	return id, false, nil
}
