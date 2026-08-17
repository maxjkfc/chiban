package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The point of the whole slice: someone else in the group sees the message
// without asking for it.
func TestAMessageReachesTheOtherMemberLive(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	kai := app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kaiSession := app.SessionCookie()

	app.SetSessionCookie(mei)
	socket := app.ConnectChat(group.ID)

	app.SetSessionCookie(kaiSession)
	sent := app.SendMessage(group.ID, "我到了")

	received := socket.Next()
	if received.ID != sent.ID {
		t.Fatalf("received id = %s, want %s", received.ID, sent.ID)
	}
	if received.Content != "我到了" {
		t.Fatalf("received content = %q, want %q", received.Content, "我到了")
	}
	if received.UserID != kai.ID {
		t.Fatalf("received user = %s, want %s", received.UserID, kai.ID)
	}

	// The broadcast must be the stored row, not the client's payload: what was
	// pushed has to be findable in history with the same identity and time.
	page := app.History(group.ID, "")
	if len(page.Messages) != 1 {
		t.Fatalf("history has %d messages, want 1", len(page.Messages))
	}
	stored := page.Messages[0]
	if stored.ID != received.ID || stored.CreatedAt != received.CreatedAt {
		t.Fatalf("broadcast %+v does not match stored %+v", received, stored)
	}
}

// A phone that loses the response and sends again must end up with one
// message, and must not make it appear twice on everyone else's screen.
func TestARetriedSendProducesOneMessage(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	socket := app.ConnectChat(group.ID)

	app.SetSessionCookie(kai)
	clientMessageID := uuid.NewString()

	first := app.PostMessage(group.ID, "我到了", clientMessageID)
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first send status = %d, want %d", first.StatusCode, http.StatusCreated)
	}
	var original testsupport.Message
	app.DecodeJSON(first, &original)

	if delivered := socket.Next(); delivered.ID != original.ID {
		t.Fatalf("delivered id = %s, want %s", delivered.ID, original.ID)
	}

	retry := app.PostMessage(group.ID, "我到了", clientMessageID)
	if retry.StatusCode != http.StatusCreated {
		t.Fatalf("retry status = %d, want %d", retry.StatusCode, http.StatusCreated)
	}
	var resent testsupport.Message
	app.DecodeJSON(retry, &resent)

	if resent.ID != original.ID {
		t.Fatalf("retry returned id %s, want the existing %s", resent.ID, original.ID)
	}
	// A retry is not news, so nobody's screen should learn about it.
	socket.ExpectSilence()

	if page := app.History(group.ID, ""); len(page.Messages) != 1 {
		t.Fatalf("history has %d messages, want 1", len(page.Messages))
	}
}

// A retry of one user's client id must not collide with another user's, or
// two people typing on freshly installed apps could silence each other.
func TestTheSameClientIDFromAnotherUserIsADifferentMessage(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)

	clientMessageID := uuid.NewString()
	mine := app.PostMessage(group.ID, "我先", clientMessageID)
	var meiMessage testsupport.Message
	app.DecodeJSON(mine, &meiMessage)

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	theirs := app.PostMessage(group.ID, "我也是", clientMessageID)
	var kaiMessage testsupport.Message
	app.DecodeJSON(theirs, &kaiMessage)

	if kaiMessage.ID == meiMessage.ID {
		t.Fatal("two users sharing a client id produced one message")
	}
	if page := app.History(group.ID, ""); len(page.Messages) != 2 {
		t.Fatalf("history has %d messages, want 2", len(page.Messages))
	}
}

// Chat is the most private thing in the product, so every way in — reading,
// writing and listening — is checked against membership, not against knowing
// the group's id.
func TestNonMembersAreShutOutOfAGroupChat(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	app.SendMessage(group.ID, "今天吃什麼")

	app.Onboard("outsider@example.com", "路人")

	if resp := app.Request(http.MethodGet, "/api/v1/groups/"+group.ID+"/messages", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if resp := app.PostMessage(group.ID, "哈囉", uuid.NewString()); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("send status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	socket, resp := app.DialChat(group.ID)
	if socket != nil {
		t.Fatal("a non-member opened the group's socket")
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("handshake status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// Leaving has to cut the live feed too, not just the REST endpoints.
func TestLeavingAGroupEndsAccessToItsChat(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	app.SendMessage(group.ID, "我到了")

	resp := app.Request(http.MethodDelete, "/api/v1/groups/"+group.ID+"/members/me", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if read := app.Request(http.MethodGet, "/api/v1/groups/"+group.ID+"/messages", nil); read.StatusCode != http.StatusForbidden {
		t.Fatalf("read after leaving = %d, want %d", read.StatusCode, http.StatusForbidden)
	}
	if socket, _ := app.DialChat(group.ID); socket != nil {
		t.Fatal("a former member opened the group's socket")
	}
}

// Scrolling up must show every message exactly once. Messages are given the
// same created_at on purpose: a cursor that only remembers the timestamp
// repeats one message and loses another at the page boundary.
func TestScrollingBackShowsEveryMessageOnce(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	const total = 5
	for i := range total {
		app.SendMessage(group.ID, string(rune('a'+i)))
	}
	if _, err := app.DB.ExecContext(t.Context(),
		`UPDATE chat_messages SET created_at = now()`); err != nil {
		t.Fatalf("collapse timestamps: %v", err)
	}

	seen := []string{}
	page := app.History(group.ID, "?limit=2")
	for {
		if len(page.Messages) == 0 {
			t.Fatal("a page came back empty before history ran out")
		}
		// Prepend: pages walk backwards, each one older than the last.
		ids := []string{}
		for _, m := range page.Messages {
			ids = append(ids, m.ID)
		}
		seen = append(ids, seen...)

		if page.Before == "" {
			break
		}
		page = app.History(group.ID, "?limit=2&before="+page.Before)
	}

	if len(seen) != total {
		t.Fatalf("saw %d messages across pages, want %d", len(seen), total)
	}
	unique := map[string]bool{}
	for _, id := range seen {
		if unique[id] {
			t.Fatalf("message %s appeared on two pages", id)
		}
		unique[id] = true
	}

	// The same messages, read in one page, must be in the same order.
	whole := app.History(group.ID, "")
	for i, m := range whole.Messages {
		if m.ID != seen[i] {
			t.Fatalf("paged order differs at %d: %s, want %s", i, seen[i], m.ID)
		}
	}
}

func TestASendWithoutUsableInputIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	cases := map[string]struct{ content, clientMessageID string }{
		"blank content":         {"   ", uuid.NewString()},
		"no client message id":  {"哈囉", ""},
		"bad client message id": {"哈囉", "not-a-uuid"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			resp := app.PostMessage(group.ID, tc.content, tc.clientMessageID)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
			}
		})
	}
}

// An opaque cursor a client made up must not be read as "start from the top",
// which would silently show the newest page while the user scrolls back.
func TestAMadeUpCursorIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	resp := app.Request(http.MethodGet, "/api/v1/groups/"+group.ID+"/messages?before=nonsense", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}
