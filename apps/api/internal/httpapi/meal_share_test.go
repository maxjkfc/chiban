package httpapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The authorization matrix for a shared meal, in one place: owner, a member of
// the group it was shared with, that same member after the share is revoked,
// and someone outside. Meal and photo are always asked together — a reader who
// can open one but not the other is the bug this is here to catch.
func TestWhoCanSeeASharedMeal(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	app.Onboard("outsider@example.com", "路人")
	outsider := app.SessionCookie()

	// Before sharing, only the owner sees it.
	app.SetSessionCookie(kai)
	if meal, photo := app.CanSeeMeal(created); meal || photo {
		t.Fatalf("a group member saw an unshared meal: meal=%v photo=%v", meal, photo)
	}

	app.SetSessionCookie(mei)
	if resp := app.ShareMeal(created.ID, group.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("share status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if meal, photo := app.CanSeeMeal(created); !meal || !photo {
		t.Fatalf("the owner lost access to their own meal: meal=%v photo=%v", meal, photo)
	}

	app.SetSessionCookie(kai)
	if meal, photo := app.CanSeeMeal(created); !meal || !photo {
		t.Fatalf("a member of the group it was shared with cannot see it: meal=%v photo=%v", meal, photo)
	}

	// Sharing with one group is not sharing with the world.
	app.SetSessionCookie(outsider)
	if meal, photo := app.CanSeeMeal(created); meal || photo {
		t.Fatalf("someone outside the group saw the meal: meal=%v photo=%v", meal, photo)
	}
}

// Revoking has to actually take access away, not just hide the meal from a
// list. Both the meal and its photos must close together.
func TestRevokingAShareEndsAccessToTheMealAndItsPhotos(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	// Prove access exists, so its absence later means something.
	if meal, photo := app.CanSeeMeal(created); !meal || !photo {
		t.Fatalf("the share never worked: meal=%v photo=%v", meal, photo)
	}

	app.SetSessionCookie(mei)
	if resp := app.UnshareMeal(created.ID, group.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unshare status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	app.SetSessionCookie(kai)
	if meal, photo := app.CanSeeMeal(created); meal || photo {
		t.Fatalf("access survived the revoke: meal=%v photo=%v", meal, photo)
	}
}

// Leaving the group ends access too: the share is to a group, not to whoever
// was in it once.
func TestLeavingTheGroupEndsAccessToItsSharedMeals(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if meal, photo := app.CanSeeMeal(created); !meal || !photo {
		t.Fatalf("the share never worked: meal=%v photo=%v", meal, photo)
	}

	if resp := app.Request(http.MethodDelete,
		"/api/v1/groups/"+group.ID+"/members/me", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if meal, photo := app.CanSeeMeal(created); meal || photo {
		t.Fatalf("a former member kept access: meal=%v photo=%v", meal, photo)
	}
}

// Sharing is the owner's decision, and only into rooms they are actually in.
func TestOnlyTheOwnerSharesAndOnlyWhereTheyBelong(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	elsewhere := app.CreateGroup("阿凱的健身群")

	// 阿凱 is in the group, but the meal is not his to share.
	if resp := app.ShareMeal(created.ID, group.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("sharing someone else's meal = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	// The refusal has to mean nothing happened, not just that the answer was
	// an error: 阿凱 is in this group, so a share that slipped through would
	// hand him the meal.
	if meal, photo := app.CanSeeMeal(created); meal || photo {
		t.Fatalf("the refused share took effect anyway: meal=%v photo=%v", meal, photo)
	}

	// 小美 owns the meal but is not in 阿凱's other group.
	app.SetSessionCookie(mei)
	if resp := app.ShareMeal(created.ID, elsewhere.ID); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("sharing into a group you are not in = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// The card must carry a reference and nothing else. A copy of the description
// in the message row would outlive the meal and drift from it.
func TestAMealMessageCopiesNothingFromTheMeal(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	created := app.UploadMeal(map[string]string{"description": "牛肉麵加辣"},
		testsupport.JPEG(t, 400, 300))
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create meal status = %d", created.StatusCode)
	}
	var meal testsupport.Meal
	app.DecodeJSON(created, &meal)

	app.ShareMeal(meal.ID, group.ID)

	var (
		messageType string
		content     *string
		mealID      *string
	)
	if err := app.DB.QueryRowContext(t.Context(), `
		SELECT message_type, content, meal_record_id::text
		FROM chat_messages WHERE group_id = $1
	`, group.ID).Scan(&messageType, &content, &mealID); err != nil {
		t.Fatalf("read the message row: %v", err)
	}

	if messageType != "meal" {
		t.Fatalf("message_type = %q, want %q", messageType, "meal")
	}
	if mealID == nil || *mealID != meal.ID {
		t.Fatalf("meal_record_id = %v, want %s", mealID, meal.ID)
	}
	if content != nil {
		t.Fatalf("the message row copied content: %q", *content)
	}
}

// Publishing to a group is the product's core loop: the card has to arrive
// without anyone reloading.
func TestASharedMealAppearsInTheGroupChatLive(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	socket := app.ConnectChat(group.ID)

	app.SetSessionCookie(mei)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)

	delivered := socket.Next()
	if delivered.Type != "meal" {
		t.Fatalf("delivered a %q message, want a meal card", delivered.Type)
	}
	if delivered.MealRecordID != created.ID {
		t.Fatalf("card points at %s, want %s", delivered.MealRecordID, created.ID)
	}
	if delivered.Content != "" {
		t.Fatalf("the card carried content: %q", delivered.Content)
	}
}

// Sharing the same meal into the same group twice must not start a second
// conversation about it.
func TestSharingTwiceDoesNotPostASecondCard(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))

	app.ShareMeal(created.ID, group.ID)
	app.ShareMeal(created.ID, group.ID)

	if page := app.History(group.ID, ""); len(page.Messages) != 1 {
		t.Fatalf("the group has %d messages, want 1", len(page.Messages))
	}

	var shares int
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM meal_group_shares WHERE meal_record_id = $1`, created.ID,
	).Scan(&shares); err != nil {
		t.Fatalf("count shares: %v", err)
	}
	if shares != 1 {
		t.Fatalf("%d share rows, want 1", shares)
	}
}

// Deleting a shared meal must not take the conversation with it: the card
// stays, the replies stay, and the meal itself simply stops being readable.
func TestDeletingASharedMealLeavesTheConversation(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)
	mei := app.SessionCookie()

	card := app.History(group.ID, "").Messages[0]

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	app.Reply(group.ID, "看起來不錯", card.ID)

	app.SetSessionCookie(mei)
	if resp := app.Request(http.MethodDelete, "/api/v1/meals/"+created.ID, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete meal status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	page := app.History(group.ID, "")
	if len(page.Messages) != 2 {
		t.Fatalf("the group has %d messages, want the card and the reply", len(page.Messages))
	}
	if page.Messages[0].ID != card.ID {
		t.Fatal("the meal card disappeared with its meal")
	}
	if page.Messages[1].Content != "看起來不錯" {
		t.Fatalf("the reply did not survive: %+v", page.Messages[1])
	}

	// The card is still there; what it points at is gone, which is how the
	// client knows to show a tombstone.
	if resp := app.ReadMeal(created.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("reading a deleted meal = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// Giving a share back after taking it away restores access without starting a
// second conversation about the same meal.
func TestResharingReopensAccessWithoutASecondCard(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	created := app.CreateMeal(testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)
	app.UnshareMeal(created.ID, group.ID)
	app.ShareMeal(created.ID, group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if meal, photo := app.CanSeeMeal(created); !meal || !photo {
		t.Fatalf("re-sharing did not reopen access: meal=%v photo=%v", meal, photo)
	}

	app.SetSessionCookie(mei)
	if page := app.History(group.ID, ""); len(page.Messages) != 1 {
		t.Fatalf("the group has %d meal cards, want 1", len(page.Messages))
	}
}

// A meal's wall-clock time belongs to whoever ate it. A lunch at 12:30 in
// Taipei reads 12:30 to everyone it is shared with, rather than sliding to
// each reader's own clock — before sharing existed, reader and owner were
// always the same person, which is what kept this hidden.
func TestASharedMealKeepsTheOwnersWallClockTime(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)

	// 04:30 UTC is 12:30 in Taipei and 23:30 the previous day in Denver.
	eatenAt := time.Date(2026, 3, 15, 4, 30, 0, 0, time.UTC)
	created := app.CreateMealAt(eatenAt, testsupport.JPEG(t, 400, 300))
	app.ShareMeal(created.ID, group.ID)

	if created.EatenAtLocal != "2026-03-15T12:30" {
		t.Fatalf("owner sees %q, want %q", created.EatenAtLocal, "2026-03-15T12:30")
	}

	app.Onboard("kai@example.com", "阿凱")
	app.SaveProfile("阿凱", "America/Denver")
	app.JoinGroup(invite.Code)

	resp := app.ReadMeal(created.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read shared meal = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var shared testsupport.Meal
	app.DecodeJSON(resp, &shared)

	if shared.EatenAtLocal != created.EatenAtLocal {
		t.Fatalf("a reader in Denver sees %q, want the owner's %q",
			shared.EatenAtLocal, created.EatenAtLocal)
	}
	if shared.IsOwner {
		t.Fatal("a shared reader is marked as the owner")
	}
}
