package group

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/database"
)

// This is the one place in the group package that tests below the HTTP seam.
//
// Redeeming an invite and revoking it are two requests racing each other, and
// the losing interleaving — the join reads a valid invite, the revoke commits,
// the join then inserts the membership — cannot be produced through a single
// HTTP request. So this drives the same store calls Join makes, from two
// transactions, and asserts the revoke cannot slip in between them.
func TestRevokeCannotCommitWhileAJoinHoldsTheInvite(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()

	ownerID := insertUser(t, db, "owner@example.com")
	joinerID := insertUser(t, db, "joiner@example.com")

	svc := NewService(db)
	g, err := svc.Create(ctx, ownerID, "午餐團")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	invite, err := svc.CreateInvite(ctx, ownerID, g.ID)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}

	// The join half, paused between reading the invite and adding the member.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	joinStore := &store{db: tx}
	if _, err := joinStore.findUsableInvite(ctx, invite.Code, time.Now()); err != nil {
		t.Fatalf("read invite: %v", err)
	}

	// The revoke half, racing the paused join.
	revoked := make(chan error, 1)
	go func() {
		revoked <- svc.RevokeInvite(context.Background(), ownerID, g.ID, invite.ID)
	}()

	select {
	case err := <-revoked:
		t.Fatalf("revoke committed while the join held the invite (err = %v); "+
			"the join would then add a member to a revoked invite", err)
	case <-time.After(500 * time.Millisecond):
		// Correct: the revoke is waiting for the join to finish.
	}

	if err := joinStore.addMember(ctx, invite.GroupID, joinerID, RoleMember); err != nil {
		t.Fatalf("add member: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-revoked; err != nil {
		t.Fatalf("revoke after the join finished: %v", err)
	}

	// And once the revoke has committed, the invite is dead for everyone else.
	if _, err := svc.Join(ctx, insertUser(t, db, "late@example.com"), invite.Code); err != ErrInviteInvalid {
		t.Fatalf("join after revoke: err = %v, want ErrInviteInvalid", err)
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
		`TRUNCATE users, groups, group_members, group_invites RESTART IDENTITY CASCADE`,
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
