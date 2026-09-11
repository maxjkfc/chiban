package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// Pinning is per-user, persisted on the backend: a second login as the same
// person, not just a second browser tab, has to see the same pinned state.
func TestPinPersistsPerUserAcrossSessions(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")

	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	got := app.GetGroup(created.ID)
	if !got.Pinned {
		t.Fatal("group not pinned right after pinning")
	}

	// A fresh login as the same user is a new session, but the same
	// backend-persisted pin must still be there.
	app.Logout()
	app.Request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "mei@example.com",
		"password": testsupport.TestPassword,
	})

	got = app.GetGroup(created.ID)
	if !got.Pinned {
		t.Fatal("pin did not survive a new login session")
	}

	list := app.ListGroups()
	if len(list) != 1 || !list[0].Pinned {
		t.Fatalf("list groups = %+v, want the one group pinned", list)
	}
}

// A pin is the caller's own bookmark, not the group's property: another
// member's pin choice must not be visible or affected.
func TestPinIsPerUserNotSharedAcrossMembers(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.PinGroup(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	got := app.GetGroup(created.ID)
	if got.Pinned {
		t.Fatal("kai sees mei's pin on the shared group")
	}
}

// Pinning a group the caller does not belong to must be refused, the same as
// every other group operation scoped by membership. Unpinning is harmless
// either way: a non-member cannot have a pin to begin with, so it stays the
// same no-op idempotent unpin gives everyone, rather than leaking group
// existence through a different status code.
func TestCannotPinAGroupYouAreNotAMemberOf(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	app.Logout()

	app.Onboard("outsider@example.com", "路人")

	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("pin status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if resp := app.UnpinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unpin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// Pinning and unpinning must both be idempotent: retrying a request that
// looks like it did not go through must not fail.
func TestPinAndUnpinAreIdempotent(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")

	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first pin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second pin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := app.GetGroup(created.ID); !got.Pinned {
		t.Fatal("group not pinned after pinning twice")
	}

	if resp := app.UnpinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("first unpin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp := app.UnpinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("second unpin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := app.GetGroup(created.ID); got.Pinned {
		t.Fatal("group still pinned after unpinning twice")
	}
}

// Unpinning a group that was never pinned is a no-op, not an error: the
// caller already has the state they want.
func TestUnpinningAnUnpinnedGroupIsHarmless(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")

	if resp := app.UnpinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unpin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := app.GetGroup(created.ID); got.Pinned {
		t.Fatal("group reports pinned after unpinning a never-pinned group")
	}
}

// Leaving a group drops the pin: rejoining later must not resurrect a stale
// bookmark from a membership that no longer exists.
func TestLeavingAGroupClearsThePin(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("pin status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	if resp := app.Request(http.MethodDelete, "/api/v1/groups/"+created.ID+"/members/me", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	rejoined := app.JoinGroup(invite.Code)
	if rejoined.Pinned {
		t.Fatal("rejoining resurrected a pin from the previous membership")
	}
}

func TestPinRequiresASession(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	app.Logout()

	if resp := app.PinGroup(created.ID); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("pin status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if resp := app.UnpinGroup(created.ID); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpin status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}
