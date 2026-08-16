package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is the slice of database/sql this package needs, so a transaction can
// be passed in later without changing anything here.
type DBTX interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// errNoRows is the package-internal signal that a lookup found nothing; the
// service translates it into a domain error before it reaches a handler.
var errNoRows = sql.ErrNoRows

type store struct {
	db DBTX
}

func (s *store) createUser(ctx context.Context, email, passwordHash string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash)
		VALUES ($1, $2)
		RETURNING id, email, created_at
	`, email, passwordHash).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err, "users_email_key") {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("auth: create user: %w", err)
	}
	return u, nil
}

func (s *store) findByEmail(ctx context.Context, email string) (User, string, error) {
	var (
		u    User
		hash string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, created_at, password_hash
		FROM users
		WHERE email = $1
	`, email).Scan(&u.ID, &u.Email, &u.CreatedAt, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", errNoRows
	}
	if err != nil {
		return User{}, "", fmt.Errorf("auth: find user by email: %w", err)
	}
	return u, hash, nil
}

func (s *store) createSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("auth: create session: %w", err)
	}
	return nil
}

func (s *store) findUserBySessionToken(ctx context.Context, tokenHash []byte, now time.Time) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2
	`, tokenHash, now).Scan(&u.ID, &u.Email, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, errNoRows
	}
	if err != nil {
		return User{}, fmt.Errorf("auth: find session: %w", err)
	}
	return u, nil
}

func (s *store) deleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("auth: delete session: %w", err)
	}
	return nil
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
