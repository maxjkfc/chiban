package chat_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/chat"
)

// Cursor encoding is the third of the three things the spec allows a unit test
// for: a pure round-trip with edge cases that are tedious to reach through
// HTTP but cheap to enumerate here.

func TestCursorSurvivesARoundTrip(t *testing.T) {
	original := chat.Cursor{
		// Microsecond precision, because that is what PostgreSQL stores.
		CreatedAt: time.Date(2026, 3, 15, 4, 5, 6, 123456000, time.UTC),
		ID:        uuid.MustParse("6b2f0b7e-2e35-4a1e-9a05-2b4b6b3a5c11"),
	}

	parsed, err := chat.ParseCursor(original.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if !parsed.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("created at = %s, want %s", parsed.CreatedAt, original.CreatedAt)
	}
	if parsed.ID != original.ID {
		t.Fatalf("id = %s, want %s", parsed.ID, original.ID)
	}
}

// Two messages sharing a timestamp must produce different cursors, or a page
// boundary between them would repeat one and skip the other.
func TestCursorsDifferWhenOnlyTheIDDiffers(t *testing.T) {
	at := time.Date(2026, 3, 15, 4, 5, 6, 0, time.UTC)

	first := chat.Cursor{CreatedAt: at, ID: uuid.New()}
	second := chat.Cursor{CreatedAt: at, ID: uuid.New()}

	if first.String() == second.String() {
		t.Fatal("messages sharing a timestamp produced the same cursor")
	}
}

// The token is opaque, so anything a client makes up must be refused rather
// than silently treated as the beginning of history.
func TestParseCursorRejectsJunk(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"not base64":        "!!!!",
		"no separator":      encodeRaw(t, "1773553506000000"),
		"bad timestamp":     encodeRaw(t, "not-a-time.6b2f0b7e-2e35-4a1e-9a05-2b4b6b3a5c11"),
		"bad uuid":          encodeRaw(t, "1773553506000000.not-a-uuid"),
		"plausible garbage": "MTIzNDU2",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := chat.ParseCursor(token); err == nil {
				t.Fatalf("accepted %q as a cursor", token)
			}
		})
	}
}

// encodeRaw builds a token the same way Cursor.String does, so these cases
// exercise the parser rather than the base64 layer.
func encodeRaw(t *testing.T, raw string) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}
