package chat

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/database"
)

// This is the one place in the chat package that tests below the HTTP seam.
//
// React reads the message, sees it is live, and then inserts. The losing
// interleaving — the read succeeds, a delete commits, the insert then lands —
// cannot be produced through a single HTTP request, because the service's own
// check refuses a deleted message long before the statement runs. What is left
// is the statement itself, and this drives it directly to prove that the guard
// in the SQL holds when the check above it has already been passed.
func TestAReactionCannotLandOnAMessageDeletedMeanwhile(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	userID := insertUser(t, db, "mei@example.com")
	groupID := insertGroup(t, db, userID)
	s := &store{db: db}

	messageID, _, err := s.insertOrGet(ctx, groupID, userID, TypeText, "今天吃什麼", uuid.New(), nil)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}

	// Stand where React stands after its check: the message was live when it
	// looked, and is deleted by the time it writes.
	message, err := s.get(ctx, messageID)
	if err != nil {
		t.Fatalf("get message: %v", err)
	}
	if message.Deleted {
		t.Fatal("the message was already deleted before the race began")
	}

	if _, err := s.softDelete(ctx, messageID); err != nil {
		t.Fatalf("delete message: %v", err)
	}

	added, err := s.addReaction(ctx, messageID, userID, Reactions[0])
	if err != nil {
		t.Fatalf("add reaction: %v", err)
	}
	if added {
		t.Fatal("a reaction landed on a message that had just been deleted")
	}

	var stored int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM message_reactions WHERE message_id = $1`, messageID,
	).Scan(&stored); err != nil {
		t.Fatalf("count reactions: %v", err)
	}
	if stored != 0 {
		t.Fatalf("the tombstone carries %d reactions, want none", stored)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv("CHIBAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("CHIBAN_TEST_DATABASE_URL is not set. Run tests with `make test`.")
	}

	db, err := database.Open(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := database.Up(t.Context(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		`TRUNCATE users, groups, group_members, chat_messages, message_reactions
		 RESTART IDENTITY CASCADE`,
	); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func insertUser(t *testing.T, db *sql.DB, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := db.QueryRowContext(t.Context(),
		`INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email,
	).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

func insertGroup(t *testing.T, db *sql.DB, ownerID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := db.QueryRowContext(t.Context(),
		`INSERT INTO groups (name, owner_id) VALUES ('午餐團', $1) RETURNING id`, ownerID,
	).Scan(&id); err != nil {
		t.Fatalf("insert group: %v", err)
	}
	return id
}

// The other half of the same race: what the caller is told.
//
// React checks the message is live and only then writes, so the losing
// interleaving cannot be arranged from outside — by the time a second request
// could delete the message, React has already refused it for the ordinary
// reason. This wraps the service's database handle to delete the message at
// exactly the moment React has read it and not yet written, and asserts the
// caller is told the reaction did not land rather than being answered "done".
func TestAReactionThatLosesTheRaceIsReportedAsRefused(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	userID := insertUser(t, db, "mei@example.com")
	groupID := insertGroup(t, db, userID)

	plain := &store{db: db}
	messageID, _, err := plain.insertOrGet(ctx, groupID, userID, TypeText, "今天吃什麼", uuid.New(), nil)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}

	racing := &deleteAfterRead{DBTX: db, db: db, messageID: messageID}
	service := NewService(racing, alwaysMember{}, NewHub())

	err = service.React(ctx, userID, messageID, Reactions[0])

	var invalid InvalidInputError
	if !errors.As(err, &invalid) {
		t.Fatalf("React returned %v, want the caller to be told it was deleted", err)
	}
	if !racing.deleted {
		t.Fatal("the message was never deleted, so this test proved nothing")
	}
}

// deleteAfterRead deletes the message once, immediately after the first read
// of it has been served. database/sql runs the query inside QueryRowContext,
// so the caller still scans the row as it was before the delete — which is the
// stale view the race is made of.
type deleteAfterRead struct {
	DBTX
	db        *sql.DB
	messageID uuid.UUID
	deleted   bool
}

func (d *deleteAfterRead) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	row := d.DBTX.QueryRowContext(ctx, query, args...)
	if !d.deleted {
		d.deleted = true
		if _, err := (&store{db: d.db}).softDelete(ctx, d.messageID); err != nil {
			panic("delete during race: " + err.Error())
		}
	}
	return row
}

type alwaysMember struct{}

func (alwaysMember) IsMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
