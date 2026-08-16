// Package auth owns user accounts and session lifecycle.
//
// Everything that decides "who is making this request" lives here. Handlers
// never read a user ID from the request body; they read it from the context
// populated by this package's middleware.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials covers both "no such email" and "wrong password",
	// deliberately: login must not reveal which accounts exist.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrEmailTaken         = errors.New("auth: email already registered")
	ErrNoSession          = errors.New("auth: no valid session")
)

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("auth: invalid %s: %s", e.Field, e.Message)
}

// SessionLifetime is how long a login lasts.
//
// ponytail: fixed lifetime, no sliding refresh. Revisit if people complain
// about being logged out mid-week.
const SessionLifetime = 30 * 24 * time.Hour

const minPasswordLength = 8

type User struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
}

// Session is a freshly minted session. Token is only ever available here, at
// creation time; the store keeps its hash.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

type Service struct {
	store *store
	// now is overridable so tests can create already-expired sessions.
	now func() time.Time
}

func NewService(db DBTX) *Service {
	return &Service{store: &store{db: db}, now: time.Now}
}

// Register creates an account and returns it. It does not log the user in;
// the caller decides whether to start a session.
func (s *Service) Register(ctx context.Context, email, password string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	if len([]rune(password)) < minPasswordLength {
		return User{}, InvalidInputError{
			Field:   "password",
			Message: fmt.Sprintf("must be at least %d characters", minPasswordLength),
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("auth: hash password: %w", err)
	}

	return s.store.createUser(ctx, email, string(hash))
}

// Authenticate verifies credentials and returns the user.
func (s *Service) Authenticate(ctx context.Context, email, password string) (User, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		// An unparseable email cannot match an account, and saying so would
		// distinguish it from a wrong password.
		return User{}, ErrInvalidCredentials
	}

	user, hash, err := s.store.findByEmail(ctx, normalized)
	if err != nil {
		if errors.Is(err, errNoRows) {
			// Still spend the time a real comparison would, so response timing
			// does not reveal whether the account exists.
			bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

// StartSession issues a new session token for the user.
func (s *Service) StartSession(ctx context.Context, userID uuid.UUID) (Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Session{}, fmt.Errorf("auth: generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := s.now().Add(SessionLifetime)

	if err := s.store.createSession(ctx, userID, hashToken(token), expiresAt); err != nil {
		return Session{}, err
	}
	return Session{Token: token, ExpiresAt: expiresAt}, nil
}

// UserForToken resolves a session token to its user, rejecting expired
// sessions. Returns ErrNoSession when the token is unusable.
func (s *Service) UserForToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNoSession
	}
	user, err := s.store.findUserBySessionToken(ctx, hashToken(token), s.now())
	if err != nil {
		if errors.Is(err, errNoRows) {
			return User{}, ErrNoSession
		}
		return User{}, err
	}
	return user, nil
}

// EndSession deletes the session. Deleting an unknown session is not an error:
// logging out twice should succeed.
func (s *Service) EndSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.deleteSession(ctx, hashToken(token))
}

func normalizeEmail(email string) (string, error) {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return "", InvalidInputError{Field: "email", Message: "is required"}
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address != trimmed {
		return "", InvalidInputError{Field: "email", Message: "is not a valid email address"}
	}
	return trimmed, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// dummyHash is a real bcrypt hash of an unguessable value, compared against
// when no account matches so that both paths cost the same.
var dummyHash = []byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy")
