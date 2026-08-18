package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

func TestUploadingAnAvatar(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	before := app.GetProfile()
	if before.AvatarMediaID != "" {
		t.Fatalf("a fresh profile already has an avatar: %q", before.AvatarMediaID)
	}

	uploaded := app.UploadAvatar(testsupport.JPEG(t, 400, 400))
	if uploaded.AvatarMediaID == "" {
		t.Fatal("upload returned no avatar media ID")
	}

	after := app.GetProfile()
	if after.AvatarMediaID != uploaded.AvatarMediaID {
		t.Fatalf("profile reports %q, upload returned %q", after.AvatarMediaID, uploaded.AvatarMediaID)
	}

	resp := app.Request(http.MethodGet, "/api/v1/avatars/"+uploaded.AvatarMediaID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("streamed an empty avatar")
	}
}

// An avatar goes through the same pipeline as a meal photo, so the same things
// are rejected: anything that is not a decodable JPEG or PNG.
func TestAvatarRejectsWhatIsNotAPhoto(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	resp := app.PostAvatar([]byte("this is not a photo"))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if app.GetProfile().AvatarMediaID != "" {
		t.Fatal("a rejected upload still set an avatar")
	}
}

// Replacing must produce a new URL, or a browser holding the old one would
// keep showing the previous picture.
func TestReplacingAnAvatarChangesItsMediaIDAndDropsTheOldObject(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	first := app.UploadAvatar(testsupport.JPEG(t, 200, 200))
	if stored := app.Storage.Len(); stored != 1 {
		t.Fatalf("%d objects stored after the first upload, want 1", stored)
	}

	second := app.UploadAvatar(testsupport.JPEG(t, 300, 300))

	if second.AvatarMediaID == first.AvatarMediaID {
		t.Fatal("replacing an avatar reused the media ID")
	}
	if stored := app.Storage.Len(); stored != 1 {
		t.Fatalf("%d objects stored after replacing, want 1: the old one should be gone", stored)
	}

	stale := app.Request(http.MethodGet, "/api/v1/avatars/"+first.AvatarMediaID, nil)
	if stale.StatusCode != http.StatusNotFound {
		t.Fatalf("the replaced media ID still resolves: status = %d, want %d",
			stale.StatusCode, http.StatusNotFound)
	}
}

// A group shows its members' names and pictures — that is the whole of what a
// group sees about someone.
func TestGroupMembersCarryTheirAvatars(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	owner := app.UploadAvatar(testsupport.JPEG(t, 200, 200))
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	resp := app.Request(http.MethodGet, "/api/v1/groups/"+created.ID+"/members", nil)
	var members []struct {
		DisplayName   string `json:"display_name"`
		AvatarMediaID string `json:"avatar_media_id"`
	}
	app.DecodeJSON(resp, &members)

	if len(members) != 2 {
		t.Fatalf("members = %+v, want 2", members)
	}
	if members[0].AvatarMediaID != owner.AvatarMediaID {
		t.Fatalf("owner avatar = %q, want %q", members[0].AvatarMediaID, owner.AvatarMediaID)
	}
	// Onboarding skips the avatar, so the second member simply has none.
	if members[1].AvatarMediaID != "" {
		t.Fatalf("member without an avatar reports %q", members[1].AvatarMediaID)
	}

	// And a fellow member can actually fetch it.
	image := app.Request(http.MethodGet, "/api/v1/avatars/"+owner.AvatarMediaID, nil)
	if image.StatusCode != http.StatusOK {
		t.Fatalf("member fetching a group-mate's avatar: status = %d, want %d",
			image.StatusCode, http.StatusOK)
	}
}

// An unguessable ID is not the authorization check. Someone who shares no
// group with the owner must be refused even holding a valid media ID, and
// leaving a group has to take effect immediately.
func TestAvatarsAreScopedToPeopleYouShareAGroupWith(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	owner := app.UploadAvatar(testsupport.JPEG(t, 200, 200))
	created := app.CreateGroup("午餐團")
	invite := app.CreateInvite(created.ID)
	app.Logout()

	// A stranger holding the ID gets the same answer as for an unknown one.
	app.Onboard("stranger@example.com", "路人")
	if resp := app.Request(http.MethodGet, "/api/v1/avatars/"+owner.AvatarMediaID, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stranger: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	app.Logout()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.Request(http.MethodGet, "/api/v1/avatars/"+owner.AvatarMediaID, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("group mate: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Leaving revokes it, without the media ID changing.
	if resp := app.Request(http.MethodDelete, "/api/v1/groups/"+created.ID+"/members/me", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp := app.Request(http.MethodGet, "/api/v1/avatars/"+owner.AvatarMediaID, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after leaving: status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestYouCanAlwaysSeeYourOwnAvatar(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	// No groups at all, so this only passes if owners are allowed outright.
	uploaded := app.UploadAvatar(testsupport.JPEG(t, 200, 200))

	resp := app.Request(http.MethodGet, "/api/v1/avatars/"+uploaded.AvatarMediaID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAvatarsRequireASession(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	uploaded := app.UploadAvatar(testsupport.JPEG(t, 200, 200))
	app.Logout()

	read := app.Request(http.MethodGet, "/api/v1/avatars/"+uploaded.AvatarMediaID, nil)
	if read.StatusCode != http.StatusUnauthorized {
		t.Fatalf("read status = %d, want %d", read.StatusCode, http.StatusUnauthorized)
	}

	upload := app.PostAvatar(testsupport.JPEG(t, 200, 200))
	if upload.StatusCode != http.StatusUnauthorized {
		t.Fatalf("upload status = %d, want %d", upload.StatusCode, http.StatusUnauthorized)
	}
}

// The client addresses avatars by ID only; there is no endpoint that takes a
// path, and the profile response never mentions where the bytes live.
func TestAvatarsAreNotAddressableByStoragePath(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")
	app.UploadAvatar(testsupport.JPEG(t, 200, 200))

	var objectName string
	if err := app.DB.QueryRowContext(t.Context(),
		`SELECT avatar_object_name FROM profiles LIMIT 1`).Scan(&objectName); err != nil {
		t.Fatalf("read object name: %v", err)
	}

	if resp := app.Request(http.MethodGet, "/api/v1/avatars/"+objectName, nil); resp.StatusCode == http.StatusOK {
		t.Fatal("a storage path resolved as an avatar ID")
	}

	resp := app.Request(http.MethodGet, "/api/v1/me/profile", nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, leaked := range []string{"avatars", "object_name", "bucket", "users/"} {
		if strings.Contains(string(body), leaked) {
			t.Fatalf("profile response leaks storage detail %q: %s", leaked, body)
		}
	}
}
