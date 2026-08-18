package chat

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Cursor marks a position in a group's history.
//
// It is the pair (created_at, id), not a timestamp alone: two messages can
// share a timestamp to the microsecond, and a cursor that only remembered the
// time would either replay them or skip them on the next page. The ID breaks
// the tie and makes the ordering total.
type Cursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// String encodes a cursor as an opaque token.
//
// Opaque because it is a position, not a contract: clients pass back what they
// were given, which leaves the encoding free to change without breaking them.
func (c Cursor) String() string {
	raw := strconv.FormatInt(c.CreatedAt.UTC().UnixMicro(), 10) + "." + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// ParseCursor reads a token produced by Cursor.String.
func ParseCursor(token string) (Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, InvalidInputError{Field: "before", Message: "is not a valid cursor"}
	}

	micros, id, found := strings.Cut(string(decoded), ".")
	if !found {
		return Cursor{}, InvalidInputError{Field: "before", Message: "is not a valid cursor"}
	}

	at, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return Cursor{}, InvalidInputError{Field: "before", Message: "is not a valid cursor"}
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return Cursor{}, InvalidInputError{Field: "before", Message: "is not a valid cursor"}
	}

	return Cursor{CreatedAt: time.UnixMicro(at).UTC(), ID: parsedID}, nil
}

func (c Cursor) isZero() bool { return c.ID == uuid.Nil }
