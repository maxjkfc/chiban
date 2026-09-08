package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The core loop the feature exists for: a new message makes a group unread
// for everyone else, and marking it read clears that.
func TestUnreadRoundTripsThroughSendAndMarkRead(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	// Fresh membership, nothing sent yet: no unread for either side.
	app.SetSessionCookie(mei)
	if g := app.GetGroup(group.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("mei's fresh group = %+v, want no unread", g)
	}

	app.SetSessionCookie(kai)
	if g := app.GetGroup(group.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("kai's fresh group = %+v, want no unread", g)
	}

	// 小美 sends; 阿凱 has not seen it yet.
	app.SetSessionCookie(mei)
	first := app.SendMessage(group.ID, "吃飯了嗎")

	app.SetSessionCookie(kai)
	if g := app.GetGroup(group.ID); !g.HasUnread || g.UnreadCount != 1 {
		t.Fatalf("kai's group after one message = %+v, want 1 unread", g)
	}
	if list := app.ListGroups(); len(list) != 1 || !list[0].HasUnread || list[0].UnreadCount != 1 {
		t.Fatalf("kai's group list = %+v, want 1 unread", list)
	}

	// A second message before 阿凱 has read anything: unread grows.
	app.SetSessionCookie(mei)
	second := app.SendMessage(group.ID, "在等你")

	app.SetSessionCookie(kai)
	if g := app.GetGroup(group.ID); g.UnreadCount != 2 {
		t.Fatalf("kai's group after two messages = %+v, want 2 unread", g)
	}

	// Marking read up to the first message leaves the second outstanding.
	if g := app.ReadGroup(group.ID, first.ID); !g.HasUnread || g.UnreadCount != 1 {
		t.Fatalf("after reading the first message = %+v, want 1 unread", g)
	}

	// Marking read up to the newest message clears it entirely.
	if g := app.ReadGroup(group.ID, second.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("after reading the newest message = %+v, want 0 unread", g)
	}
	if g := app.GetGroup(group.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("kai's group after marking read = %+v, want 0 unread", g)
	}

	// The sender's own group must never show their own message as unread.
	app.SetSessionCookie(mei)
	if g := app.GetGroup(group.ID); g.HasUnread {
		t.Fatalf("mei's group = %+v, sender should not see their own message as unread", g)
	}
}

// A message the reader sends after marking read must not resurrect it as
// unread for themselves, and must show as unread for everyone else who has
// not read up to it yet.
func TestSendingAfterMarkingReadStaysRead(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	app.SendMessage(group.ID, "第一句")
	latest := app.SendMessage(group.ID, "第二句")
	app.ReadGroup(group.ID, latest.ID)

	if g := app.GetGroup(group.ID); g.HasUnread {
		t.Fatalf("sender's group after reading their own messages = %+v, want no unread", g)
	}

	app.SendMessage(group.ID, "第三句")
	if g := app.GetGroup(group.ID); g.HasUnread {
		t.Fatalf("sender's group after sending a new message = %+v, want no unread", g)
	}
}

// Marking read must never accept a message from another group: that would
// let a member forge having read messages they were never shown, or read
// state leaking across a private boundary.
func TestMarkReadRejectsAMessageFromAnotherGroup(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	lunch := app.CreateGroup("午餐團")
	dinner := app.CreateGroup("晚餐團")

	foreign := app.SendMessage(dinner.ID, "晚餐吃什麼")

	resp := app.MarkGroupRead(lunch.ID, foreign.ID)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// A non-member must not be able to mark a group read: not the messages, not
// even the read cursor itself.
func TestNonMemberCannotMarkAGroupRead(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "哈囉")

	app.Onboard("outsider@example.com", "路人")
	resp := app.MarkGroupRead(group.ID, message.ID)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// A made-up message id must not silently mark everything read, or a client
// bug could hide real unread messages from the user.
func TestMarkReadRejectsAnUnknownMessageID(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	resp := app.MarkGroupRead(group.ID, uuid.NewString())
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestMarkReadRejectsAMalformedMessageID(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	resp := app.MarkGroupRead(group.ID, "not-a-uuid")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// A deleted message is still a real position in the conversation: marking
// read up to a tombstone must succeed and count the tombstone itself as
// read, matching the spec's rule that soft-deleted messages still count.
func TestMarkReadAcceptsATombstonedMessage(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	toDelete := app.SendMessage(group.ID, "刪我")
	if resp := app.DeleteMessage(toDelete.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	app.SetSessionCookie(kai)
	if g := app.GetGroup(group.ID); g.UnreadCount != 1 {
		t.Fatalf("unread before marking the tombstone read = %+v, want 1", g)
	}

	if g := app.ReadGroup(group.ID, toDelete.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("after reading up to a tombstone = %+v, want 0 unread", g)
	}
}

// Two groups' unread state must not bleed into each other: a member unread in
// one group and current in another must see exactly that split.
func TestUnreadIsScopedPerGroup(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	lunch := app.CreateGroup("午餐團")
	dinner := app.CreateGroup("晚餐團")
	lunchInvite := app.CreateInvite(lunch.ID)
	dinnerInvite := app.CreateInvite(dinner.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(lunchInvite.Code)
	app.JoinGroup(dinnerInvite.Code)
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	lunchMessage := app.SendMessage(lunch.ID, "午餐吃什麼")
	app.SendMessage(dinner.ID, "晚餐吃什麼")

	app.SetSessionCookie(kai)
	app.ReadGroup(lunch.ID, lunchMessage.ID)

	byID := map[string]testsupport.Group{}
	for _, g := range app.ListGroups() {
		byID[g.ID] = g
	}
	if g := byID[lunch.ID]; g.HasUnread {
		t.Fatalf("lunch group = %+v, want read", g)
	}
	if g := byID[dinner.ID]; !g.HasUnread || g.UnreadCount != 1 {
		t.Fatalf("dinner group = %+v, want 1 unread", g)
	}
}

// A stale client replaying an older "mark read" after the user already read
// further must not resurrect the messages in between as unread again.
func TestMarkReadCannotMoveBackward(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	first := app.SendMessage(group.ID, "第一句")
	second := app.SendMessage(group.ID, "第二句")

	app.ReadGroup(group.ID, second.ID)
	if g := app.GetGroup(group.ID); g.HasUnread {
		t.Fatalf("after reading the newest message = %+v, want no unread", g)
	}

	// Replaying an older cursor must not undo the newer one.
	if g := app.ReadGroup(group.ID, first.ID); g.HasUnread || g.UnreadCount != 0 {
		t.Fatalf("after replaying an older read = %+v, want still 0 unread", g)
	}
}
