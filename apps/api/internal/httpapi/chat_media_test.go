package httpapi_test

import (
	"bytes"
	"image/gif"
	"io"
	"net/http"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// An image posted into a group reaches the other member live, and the bytes
// behind it are readable by them — the card is worthless if the picture 404s.
func TestAnImageReachesTheGroupAndItsBytesAreReadable(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	socket := app.ConnectChat(group.ID)
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 400, 300), "lunch.jpg")
	if uploaded.Type != "image" {
		t.Fatalf("media type = %q, want %q", uploaded.Type, "image")
	}
	if resp := app.SendMedia(group.ID, uploaded.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send media: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	delivered := socket.Next()
	if delivered.Type != "image" {
		t.Fatalf("delivered a %q message, want an image", delivered.Type)
	}
	if delivered.ChatMediaID != uploaded.ID {
		t.Fatalf("message points at %s, want %s", delivered.ChatMediaID, uploaded.ID)
	}
	if delivered.Content != "" {
		t.Fatalf("an image message carried content %q", delivered.Content)
	}

	app.SetSessionCookie(kai)
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("the recipient cannot read the image: %d", resp.StatusCode)
	}
}

// A GIF has to come back animated. Re-encoding it as a still would be a silent
// downgrade — the file still loads, it just stops moving.
func TestAGIFKeepsItsFramesThroughTheChat(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	uploaded := app.UploadChatMedia(testsupport.AnimatedGIF(t, 40, 40, 6), "wave.gif")
	if uploaded.Type != "gif" {
		t.Fatalf("media type = %q, want %q", uploaded.Type, "gif")
	}
	if resp := app.SendMedia(group.ID, uploaded.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send gif: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	resp := app.ReadChatMedia(uploaded.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read gif: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/gif" {
		t.Fatalf("content type = %q, want image/gif", got)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("what came back is not a GIF: %v", err)
	}
	if len(decoded.Image) != 6 {
		t.Fatalf("%d frames survived, want 6", len(decoded.Image))
	}
}

// Posting is what widens who can read an upload. Before that it is the
// uploader's alone, and afterwards only the group it was posted into.
func TestWhoCanReadChatMedia(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 200, 200), "lunch.jpg")
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	// Uploaded but not posted: nobody else can see it yet.
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("an unposted upload was readable by someone else: %d", resp.StatusCode)
	}

	app.SetSessionCookie(mei)
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("the uploader cannot read their own upload: %d", resp.StatusCode)
	}
	app.SendMedia(group.ID, uploaded.ID)

	app.SetSessionCookie(kai)
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("a member of the group it was posted in cannot read it: %d", resp.StatusCode)
	}

	app.Onboard("outsider@example.com", "路人")
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("someone outside the group read the image: %d", resp.StatusCode)
	}
}

// Posting someone else's upload would hand it to a group its uploader never
// chose, so the sender has to own what they post.
func TestPostingSomeoneElsesUploadIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 200, 200), "lunch.jpg")

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	if resp := app.SendMedia(group.ID, uploaded.ID); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("posting someone else's upload = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if page := app.History(group.ID, ""); len(page.Messages) != 0 {
		t.Fatalf("the refused post left %d messages", len(page.Messages))
	}
}

// The message row must carry a reference and nothing about where the bytes
// live, so changing storage layout never breaks messages already sent.
func TestAMediaMessageCarriesNoObjectPath(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 200, 200), "lunch.jpg")
	app.SendMedia(group.ID, uploaded.ID)

	var (
		messageType string
		content     *string
		mediaID     *string
	)
	if err := app.DB.QueryRowContext(t.Context(), `
		SELECT message_type, content, chat_media_id::text
		FROM chat_messages WHERE group_id = $1
	`, group.ID).Scan(&messageType, &content, &mediaID); err != nil {
		t.Fatalf("read the message row: %v", err)
	}

	if messageType != "image" {
		t.Fatalf("message_type = %q, want %q", messageType, "image")
	}
	if mediaID == nil || *mediaID != uploaded.ID {
		t.Fatalf("chat_media_id = %v, want %s", mediaID, uploaded.ID)
	}
	if content != nil {
		t.Fatalf("the message row carried content %q", *content)
	}
}

func TestAnUploadThatIsNotAnImageIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")

	if resp := app.PostChatMedia([]byte("not an image at all"), "notes.txt"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// Deleting the message takes the picture with it. The tombstone says the
// content is gone, so the bytes it pointed at must stop being readable too —
// otherwise anyone who noted the media id keeps the image forever.
func TestDeletingAMediaMessageEndsAccessToItsBytes(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 200, 200), "lunch.jpg")
	app.SendMedia(group.ID, uploaded.ID)
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("the image was never readable: %d", resp.StatusCode)
	}
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	message := app.History(group.ID, "").Messages[0]
	if resp := app.DeleteMessage(message.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	app.SetSessionCookie(kai)
	if resp := app.ReadChatMedia(uploaded.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("the image outlived its message: %d", resp.StatusCode)
	}
}
