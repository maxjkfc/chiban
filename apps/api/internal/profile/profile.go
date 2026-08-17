// Package profile owns the small amount of identity a group sees: display
// name and the timezone that decides where the user's day starts and ends.
//
// V0.1 deliberately stores nothing else. Height, weight, sex and calorie
// targets belong to a later version and are not collected here.
package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	// Embeds the IANA timezone database in the binary. The API runs on a
	// distroless image with no system tzdata, so without this every
	// LoadLocation call would fail there but succeed on a developer's Mac.
	_ "time/tzdata"

	"github.com/google/uuid"
)

// ErrNotFound means the user has not completed onboarding yet.
var ErrNotFound = errors.New("profile: not found")

// InvalidInputError describes input the caller can fix.
type InvalidInputError struct {
	Field   string
	Message string
}

func (e InvalidInputError) Error() string {
	return fmt.Sprintf("profile: invalid %s: %s", e.Field, e.Message)
}

const maxDisplayNameLength = 50

type Profile struct {
	UserID      uuid.UUID
	DisplayName string
	Timezone    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Input carries a partial update. A nil field is left untouched, which is what
// makes PATCH semantics work; both are required when the profile is created.
type Input struct {
	DisplayName *string
	Timezone    *string
}

type Service struct {
	store *store
}

func NewService(db DBTX) *Service {
	return &Service{store: &store{db: db}}
}

func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Profile, error) {
	return s.store.find(ctx, userID)
}

// Save creates the profile on first call and updates it afterwards, so
// onboarding and later edits are the same operation.
func (s *Service) Save(ctx context.Context, userID uuid.UUID, in Input) (Profile, error) {
	current, err := s.store.find(ctx, userID)
	creating := errors.Is(err, ErrNotFound)
	if err != nil && !creating {
		return Profile{}, err
	}

	displayName := current.DisplayName
	if in.DisplayName != nil {
		displayName = strings.TrimSpace(*in.DisplayName)
	}
	timezone := current.Timezone
	if in.Timezone != nil {
		timezone = strings.TrimSpace(*in.Timezone)
	}

	if err := validateDisplayName(displayName); err != nil {
		return Profile{}, err
	}
	if err := validateTimezone(timezone); err != nil {
		return Profile{}, err
	}

	return s.store.upsert(ctx, userID, displayName, timezone)
}

func validateDisplayName(name string) error {
	if name == "" {
		return InvalidInputError{Field: "display_name", Message: "is required"}
	}
	if len([]rune(name)) > maxDisplayNameLength {
		return InvalidInputError{
			Field:   "display_name",
			Message: fmt.Sprintf("must be at most %d characters", maxDisplayNameLength),
		}
	}
	return nil
}

// validateTimezone insists on a real IANA name. Storing an unloadable zone
// would break every Today and history query for that user later on.
func validateTimezone(name string) error {
	if name == "" {
		return InvalidInputError{Field: "timezone", Message: "is required"}
	}
	// LoadLocation accepts "Local", but that would make the user's day
	// boundary depend on whichever machine serves the request. UTC is fine.
	if name == "Local" {
		return InvalidInputError{Field: "timezone", Message: "must be a specific IANA timezone"}
	}
	if _, err := time.LoadLocation(name); err != nil {
		return InvalidInputError{Field: "timezone", Message: "is not a valid IANA timezone"}
	}
	return nil
}
