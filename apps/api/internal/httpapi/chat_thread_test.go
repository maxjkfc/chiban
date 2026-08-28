package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

const (
	heart = "❤️"
	laugh = "😂"
)

// A reply has to arrive carrying what it answers, or the reader sees an
// out-of-context line and has to go hunting for the message above it.
func TestAReplyQuotesTheMessageItAnswers(t *testing.T) {
	app := testsupport.NewApp(t)

	mei := app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	original := app.SendMessage(group.ID, "今天吃什麼")

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	reply := app.Reply(group.ID, "牛肉麵", original.ID)

	if reply.ReplyTo == nil {
		t.Fatal("the reply came back without the message it answers")
	}
	if reply.ReplyTo.ID != original.ID {
		t.Fatalf("quoted id = %s, want %s", reply.ReplyTo.ID, original.ID)
	}
	if reply.ReplyTo.Content != "今天吃什麼" {
		t.Fatalf("quoted content = %q, want %q", reply.ReplyTo.Content, "今天吃什麼")
	}
	if reply.ReplyTo.UserID != mei.ID {
		t.Fatalf("quoted author = %s, want %s", reply.ReplyTo.UserID, mei.ID)
	}

	// The quote is joined at read time, so history says the same thing.
	page := app.History(group.ID, "")
	if len(page.Messages) != 2 {
		t.Fatalf("history has %d messages, want 2", len(page.Messages))
	}
	if quoted := page.Messages[1].ReplyTo; quoted == nil || quoted.ID != original.ID {
		t.Fatalf("history lost the quote: %+v", page.Messages[1])
	}
}

// Replying across a boundary would put a message you cannot read into a quote
// you can, so the parent has to belong to the conversation you are in.
func TestAReplyCannotPointAtAnotherGroupsMessage(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	lunch := app.CreateGroup("午餐團")
	dinner := app.CreateGroup("晚餐團")
	elsewhere := app.SendMessage(dinner.ID, "晚上吃鍋")

	resp := app.PostReply(lunch.ID, "咦", elsewhere.ID)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	resp = app.PostReply(lunch.ID, "咦", uuid.NewString())
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown parent status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// Only the author gets to remove a message, and removing it turns it into a
// tombstone rather than a hole in the conversation.
func TestOnlyTheAuthorCanDeleteAMessage(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	original := app.SendMessage(group.ID, "今天吃什麼")
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	reply := app.Reply(group.ID, "牛肉麵", original.ID)

	// 阿凱 is in the group and can see the message, but it is not his to remove.
	if resp := app.DeleteMessage(original.ID); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("deleting someone else's message = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	app.SetSessionCookie(mei)
	if resp := app.DeleteMessage(original.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	page := app.History(group.ID, "")
	if len(page.Messages) != 2 {
		t.Fatalf("history has %d messages, want the tombstone and the reply", len(page.Messages))
	}

	tombstone := page.Messages[0]
	if !tombstone.Deleted {
		t.Fatal("the deleted message does not say it was deleted")
	}
	if tombstone.Content != "" {
		t.Fatalf("the deleted message still carries %q", tombstone.Content)
	}

	standing := page.Messages[1]
	if standing.ID != reply.ID || standing.Content != "牛肉麵" {
		t.Fatalf("the reply did not survive its parent: %+v", standing)
	}
	if standing.ReplyTo == nil || !standing.ReplyTo.Deleted {
		t.Fatalf("the reply's quote should read as deleted: %+v", standing.ReplyTo)
	}
	if standing.ReplyTo.Content != "" {
		t.Fatalf("the quote still carries the removed content %q", standing.ReplyTo.Content)
	}
}

// A double tap on a phone is one reaction, not two. The count is only worth
// showing if it cannot be inflated by repeating yourself.
func TestReactingTwiceCountsOnce(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "今天吃什麼")

	app.React(message.ID, heart)
	app.React(message.ID, heart)

	page := app.History(group.ID, "")
	tally, ok := page.Messages[0].Find(heart)
	if !ok {
		t.Fatal("the reaction did not stick")
	}
	if tally.Count != 1 {
		t.Fatalf("count = %d, want 1", tally.Count)
	}
	if !tally.Mine {
		t.Fatal("the reader's own reaction is not marked as theirs")
	}
}

// Two people reacting is two, and each of them should only see their own as
// theirs — that is what makes tapping again the way to take it back.
func TestReactionsAreCountedPerPerson(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	message := app.SendMessage(group.ID, "今天吃什麼")
	app.React(message.ID, heart)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	app.React(message.ID, heart)
	app.React(message.ID, laugh)

	tally, _ := app.History(group.ID, "").Messages[0].Find(heart)
	if tally.Count != 2 || !tally.Mine {
		t.Fatalf("kai sees %+v, want count 2 and mine", tally)
	}

	// 小美 never reacted with 😂, so it must not be marked as hers.
	app.SetSessionCookie(mei)
	message = app.History(group.ID, "").Messages[0]
	if tally, _ = message.Find(laugh); tally.Count != 1 || tally.Mine {
		t.Fatalf("mei sees %+v for 😂, want count 1 and not mine", tally)
	}
}

// Taking back a reaction removes yours and only yours.
func TestRemovingAReactionLeavesTheOthersAlone(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	message := app.SendMessage(group.ID, "今天吃什麼")
	app.React(message.ID, heart)

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	app.React(message.ID, heart)

	if resp := app.Unreact(message.ID, heart); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unreact status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	tally, ok := app.History(group.ID, "").Messages[0].Find(heart)
	if !ok {
		t.Fatal("removing one reaction removed the other person's too")
	}
	if tally.Count != 1 {
		t.Fatalf("count = %d, want 1", tally.Count)
	}
	if tally.Mine {
		t.Fatal("the reaction still reads as the caller's after they took it back")
	}
}

// Reacting is writing into a group, so it is gated on membership exactly like
// sending is — knowing a message id is not permission.
func TestNonMembersCannotReplyOrReact(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "今天吃什麼")

	app.Onboard("outsider@example.com", "路人")

	if resp := app.PostReply(group.ID, "哈囉", message.ID); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("reply status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if resp := app.PostReaction(message.ID, heart); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("react status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if resp := app.Unreact(message.ID, heart); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("unreact status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestAnInvalidReactionIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "今天吃什麼")

	// Custom emojis (like pizza) are now valid reactions
	if resp := app.PostReaction(message.ID, "🍕"); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("custom emoji reaction status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Empty string is refused
	if resp := app.PostReaction(message.ID, ""); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty reaction status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Whitespace only is refused
	if resp := app.PostReaction(message.ID, "   "); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("whitespace reaction status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Too long (>16 bytes) is refused
	if resp := app.PostReaction(message.ID, "this_is_way_too_long_for_an_emoji"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("too long reaction status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// A reaction is only useful if the other side sees it without asking, the same
// way a message is.
func TestAReactionReachesTheOtherMemberLive(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	message := app.SendMessage(group.ID, "今天吃什麼")
	mei := app.SessionCookie()

	kai := app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kaiSession := app.SessionCookie()

	app.SetSessionCookie(mei)
	socket := app.ConnectChat(group.ID)

	app.SetSessionCookie(kaiSession)
	app.React(message.ID, heart)

	event := socket.NextEvent()
	if event.Type != "reaction" || event.Reaction == nil {
		t.Fatalf("got a %q event, want a reaction", event.Type)
	}
	if event.Reaction.MessageID != message.ID {
		t.Fatalf("reaction on %s, want %s", event.Reaction.MessageID, message.ID)
	}
	if event.Reaction.UserID != kai.ID {
		t.Fatalf("reaction by %s, want %s", event.Reaction.UserID, kai.ID)
	}
	if event.Reaction.Type != heart || !event.Reaction.Added {
		t.Fatalf("reaction = %+v, want %s added", event.Reaction, heart)
	}

	// Repeating it changes nothing, so nobody's screen should hear about it.
	app.React(message.ID, heart)
	socket.ExpectSilence()
}

// A deletion has to reach the other screens too, or a message stays readable
// on them after it was removed.
func TestADeletionReachesTheOtherMemberLive(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	message := app.SendMessage(group.ID, "打錯了")
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	socket := app.ConnectChat(group.ID)

	app.SetSessionCookie(kai)
	if resp := app.DeleteMessage(message.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	event := socket.NextEvent()
	if event.Type != "deleted" {
		t.Fatalf("got a %q event, want a deletion", event.Type)
	}
	if event.MessageID != message.ID {
		t.Fatalf("deleted %s, want %s", event.MessageID, message.ID)
	}
}

// Reactions are answers to content. When the content goes, they go with it —
// unlike replies, which are their author's own contribution and stay.
func TestDeletingAMessageTakesItsReactionsWithIt(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "今天吃什麼")
	app.React(message.ID, heart)

	if resp := app.DeleteMessage(message.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	tombstone := app.History(group.ID, "").Messages[0]
	if !tombstone.Deleted {
		t.Fatal("the message was not tombstoned")
	}
	if len(tombstone.Reactions) != 0 {
		t.Fatalf("the tombstone still carries %+v", tombstone.Reactions)
	}
}

// There is nothing left to react to, and a reaction that outlived its message
// would reappear under the tombstone on every reload.
func TestReactingToADeletedMessageIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	message := app.SendMessage(group.ID, "今天吃什麼")
	app.DeleteMessage(message.ID)

	if resp := app.PostReaction(message.ID, heart); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if len(app.History(group.ID, "").Messages[0].Reactions) != 0 {
		t.Fatal("a reaction landed on a deleted message")
	}
}
