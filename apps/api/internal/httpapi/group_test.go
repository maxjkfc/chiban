package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The whole product starts here: someone makes a group and a friend joins it
// through a link.
func TestInviteLetsASecondAccountJoin(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	joined := app.JoinGroup(invite.Code)

	if joined.ID != created.ID {
		t.Fatalf("joined group %s, want %s", joined.ID, created.ID)
	}
	if joined.IsOwner {
		t.Fatal("the joiner must not become the owner")
	}

	resp := app.Request(http.MethodGet, "/api/v1/groups", nil)
	var groups []testsupport.Group
	app.DecodeJSON(resp, &groups)
	if len(groups) != 1 || groups[0].ID != created.ID {
		t.Fatalf("kai's groups = %+v, want just the joined group", groups)
	}
}

// A private group is only private if non-members are refused by the backend.
func TestNonMembersCannotReadAGroup(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	app.Logout()

	app.Onboard("outsider@example.com", "路人")

	for _, path := range []string{
		"/api/v1/groups/" + created.ID,
		"/api/v1/groups/" + created.ID + "/members",
	} {
		resp := app.Request(http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("GET %s: status = %d, want %d", path, resp.StatusCode, http.StatusForbidden)
		}
	}

	invite := app.Request(http.MethodPost, "/api/v1/groups/"+created.ID+"/invites", nil)
	if invite.StatusCode != http.StatusForbidden {
		t.Fatalf("invite status = %d, want %d", invite.StatusCode, http.StatusForbidden)
	}

	list := app.Request(http.MethodGet, "/api/v1/groups", nil)
	var groups []testsupport.Group
	app.DecodeJSON(list, &groups)
	if len(groups) != 0 {
		t.Fatalf("outsider sees %d groups, want 0", len(groups))
	}
}

func TestMembersListShowsDisplayNames(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	resp := app.Request(http.MethodGet, "/api/v1/groups/"+created.ID+"/members", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var members []struct {
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	app.DecodeJSON(resp, &members)

	if len(members) != 2 {
		t.Fatalf("members = %+v, want 2", members)
	}
	if members[0].DisplayName != "小美" || members[0].Role != "owner" {
		t.Fatalf("first member = %+v, want 小美 as owner", members[0])
	}
	if members[1].DisplayName != "阿凱" || members[1].Role != "member" {
		t.Fatalf("second member = %+v, want 阿凱 as member", members[1])
	}
}

// A leaked link has to be killable, and a killed link must stop working.
func TestRevokedInviteStopsWorking(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)

	revoke := app.Request(http.MethodDelete,
		"/api/v1/groups/"+created.ID+"/invites/"+invite.ID, nil)
	if revoke.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want %d", revoke.StatusCode, http.StatusNoContent)
	}
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	resp := app.Request(http.MethodPost, "/api/v1/groups/join", map[string]string{"code": invite.Code})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("join status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestExpiredInviteStopsWorking(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)

	if _, err := app.DB.ExecContext(t.Context(),
		`UPDATE group_invites SET expires_at = now() - interval '1 second'`,
	); err != nil {
		t.Fatalf("expire invite: %v", err)
	}
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	resp := app.Request(http.MethodPost, "/api/v1/groups/join", map[string]string{"code": invite.Code})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("join status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestUnknownInviteCodeIsRejected(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := app.Request(http.MethodPost, "/api/v1/groups/join", map[string]string{"code": "nosuchcode"})

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// Clicking a link twice is the natural response to a page that looks stuck, so
// it must not fail or create a second membership.
func TestJoiningTwiceIsHarmless(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	app.JoinGroup(invite.Code)

	var memberships int
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT count(*) FROM group_members WHERE group_id = $1`, created.ID,
	).Scan(&memberships); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if memberships != 2 {
		t.Fatalf("group has %d memberships, want 2", memberships)
	}
}

// Re-using an invite must never demote the owner to an ordinary member.
func TestOwnerRejoiningKeepsOwnership(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)

	rejoined := app.JoinGroup(invite.Code)

	if !rejoined.IsOwner || rejoined.Role != "owner" {
		t.Fatalf("owner became %+v after re-joining", rejoined)
	}
}

// V0.1 has no ownership transfer, so refusing the owner is what keeps a group
// from ending up with nobody in charge.
func TestOwnerCannotLeaveButMembersCan(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)

	ownerLeave := app.Request(http.MethodDelete, "/api/v1/groups/"+created.ID+"/members/me", nil)
	if ownerLeave.StatusCode != http.StatusConflict {
		t.Fatalf("owner leave status = %d, want %d", ownerLeave.StatusCode, http.StatusConflict)
	}
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	memberLeave := app.Request(http.MethodDelete, "/api/v1/groups/"+created.ID+"/members/me", nil)
	if memberLeave.StatusCode != http.StatusNoContent {
		t.Fatalf("member leave status = %d, want %d", memberLeave.StatusCode, http.StatusNoContent)
	}

	after := app.Request(http.MethodGet, "/api/v1/groups/"+created.ID, nil)
	if after.StatusCode != http.StatusForbidden {
		t.Fatalf("after leaving, status = %d, want %d", after.StatusCode, http.StatusForbidden)
	}
}

// Only the owner may revoke, so one member cannot undo another's invite.
func TestOnlyTheOwnerCanRevokeAnInvite(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	resp := app.Request(http.MethodDelete,
		"/api/v1/groups/"+created.ID+"/invites/"+invite.ID, nil)

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestGroupRequiresASession(t *testing.T) {
	app := testsupport.NewApp(t)

	create := app.Request(http.MethodPost, "/api/v1/groups", map[string]string{"name": "午餐團"})
	if create.StatusCode != http.StatusUnauthorized {
		t.Fatalf("create status = %d, want %d", create.StatusCode, http.StatusUnauthorized)
	}

	join := app.Request(http.MethodPost, "/api/v1/groups/join", map[string]string{"code": "whatever"})
	if join.StatusCode != http.StatusUnauthorized {
		t.Fatalf("join status = %d, want %d", join.StatusCode, http.StatusUnauthorized)
	}
}

func TestCreateGroupRejectsInvalidNames(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	for _, name := range []string{"", "   "} {
		resp := app.Request(http.MethodPost, "/api/v1/groups", map[string]string{"name": name})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("name %q: status = %d, want %d", name, resp.StatusCode, http.StatusBadRequest)
		}
	}
}
