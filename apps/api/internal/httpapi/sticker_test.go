package httpapi_test

import (
	"bytes"
	"image/gif"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The whole loop a sticker exists for: keep one, find it again, send it.
func TestAStickerCanBeKeptListedAndSent(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	if added.Type != "image" {
		t.Fatalf("sticker type = %q, want %q", added.Type, "image")
	}

	library := app.ListStickers()
	if len(library) != 1 || library[0].ID != added.ID {
		t.Fatalf("library = %+v, want the one sticker just added", library)
	}

	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send sticker: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	page := app.History(group.ID, "")
	if len(page.Messages) != 1 {
		t.Fatalf("%d messages in history, want 1", len(page.Messages))
	}
	sent := page.Messages[0]
	// The kind is its own, not "image": what a reader does with a sticker and
	// with a posted photo are different things.
	if sent.Type != "sticker" {
		t.Fatalf("message type = %q, want %q", sent.Type, "sticker")
	}
	if sent.StickerID != added.ID {
		t.Fatalf("message sticker id = %q, want %q", sent.StickerID, added.ID)
	}
	if sent.Content != "" {
		t.Fatalf("a sticker message carried content %q", sent.Content)
	}
}

// A sticker is worth having as a GIF only if it still moves. Re-encoding it to
// a single frame would leave something that looks right in the picker and is
// wrong in the conversation.
func TestAGIFStickerKeepsItsFrames(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	added := app.AddSticker(testsupport.AnimatedGIF(t, 40, 40, 5), "wave.gif")
	if added.Type != "gif" {
		t.Fatalf("sticker type = %q, want %q", added.Type, "gif")
	}
	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send gif sticker: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	resp := app.ReadSticker(added.ID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read sticker: status = %d, want %d", resp.StatusCode, http.StatusOK)
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
	if len(decoded.Image) != 5 {
		t.Fatalf("%d frames survived, want 5", len(decoded.Image))
	}
}

// Story 69: a library is the owner's own. Someone else's delete must not
// reach into it, and the refusal must not confirm the sticker exists either.
func TestOnlyTheOwnerCanDeleteASticker(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.DeleteSticker(added.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting someone else's sticker = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// The refusal has to have refused: a response code alone would pass even
	// if the row had been deleted anyway.
	app.SetSessionCookie(mei)
	if library := app.ListStickers(); len(library) != 1 {
		t.Fatalf("the owner's library has %d stickers, want 1", len(library))
	}
}

// Sending someone else's sticker would let anyone paste any sticker id and
// push another person's picture into a group.
func TestSendingSomeoneElsesStickerIsRefused(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)

	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("sending someone else's sticker = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if page := app.History(group.ID, ""); len(page.Messages) != 0 {
		t.Fatalf("the refused send left %d messages", len(page.Messages))
	}
}

// Deleting is for tidying the picker, not for retracting what was already
// said. The messages keep their sticker and the bytes keep loading, which is
// what "no broken images" means once a sticker is gone.
func TestDeletingAStickerLeavesTheMessagesItAlreadySent(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send sticker: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	kai := app.SessionCookie()

	app.SetSessionCookie(mei)
	if resp := app.DeleteSticker(added.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete sticker: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if library := app.ListStickers(); len(library) != 0 {
		t.Fatalf("a deleted sticker is still in the picker: %+v", library)
	}

	// The message still points at it, and the bytes still load — for the
	// people who were there, not only for the owner.
	app.SetSessionCookie(kai)
	page := app.History(group.ID, "")
	if len(page.Messages) != 1 || page.Messages[0].StickerID != added.ID {
		t.Fatalf("the sticker message did not survive its sticker: %+v", page.Messages)
	}
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("reading a deleted sticker already sent = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// It cannot be sent again, though: it is out of the library.
	app.SetSessionCookie(mei)
	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("sending a deleted sticker = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// A sticker is a personal picture, so it reaches exactly the people its owner
// shares a group with — the same rule an avatar follows.
func TestWhoCanReadASticker(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	mei := app.SessionCookie()

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("someone in a group with the owner cannot read it: %d", resp.StatusCode)
	}

	app.Onboard("outsider@example.com", "路人")
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("someone sharing no group with the owner read it: %d", resp.StatusCode)
	}

	app.SetSessionCookie(mei)
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("the owner cannot read their own sticker: %d", resp.StatusCode)
	}
}

// Leaving takes the picture with it: viewership is asked at read time, not
// remembered from when the sticker was sent.
func TestLeavingTheGroupEndsAccessToItsStickers(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	invite := app.CreateInvite(group.ID)
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")

	app.Onboard("kai@example.com", "阿凱")
	app.JoinGroup(invite.Code)
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusOK {
		t.Fatalf("a member cannot read the sticker: %d", resp.StatusCode)
	}

	if resp := app.Request(http.MethodDelete,
		"/api/v1/groups/"+group.ID+"/members/me", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("leave status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if resp := app.ReadSticker(added.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a former member read the sticker: %d", resp.StatusCode)
	}
}

// One message shows one thing. Accepting both would make the message type a
// guess about which of them the reader is meant to see.
func TestAMessageCannotBeAStickerAndAnUploadAtOnce(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	uploaded := app.UploadChatMedia(testsupport.JPEG(t, 200, 200), "lunch.jpg")

	resp := app.Request(http.MethodPost, "/api/v1/groups/"+group.ID+"/messages", map[string]string{
		"client_message_id": uuid.NewString(),
		"sticker_id":        added.ID,
		"chat_media_id":     uploaded.ID,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("sticker and upload together = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if page := app.History(group.ID, ""); len(page.Messages) != 0 {
		t.Fatalf("the refused send left %d messages", len(page.Messages))
	}
}

// The sticker message carries a reference and nothing about where the bytes
// live, so changing storage layout never breaks messages already sent.
func TestAStickerMessageCarriesNoObjectPath(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")
	added := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")
	if resp := app.SendSticker(group.ID, added.ID); resp.StatusCode != http.StatusCreated {
		t.Fatalf("send sticker: status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	resp := app.Request(http.MethodGet, "/api/v1/groups/"+group.ID+"/messages", nil)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, leak := range []string{"stickers/", "bucket", "object_name"} {
		if bytes.Contains(body, []byte(leak)) {
			t.Fatalf("the message payload contains %q: %s", leak, body)
		}
	}
}

// pinnedRail reads the quick rail back out of a library: the pinned stickers,
// in slot order. The library itself stays newest-first, so the rail is only
// ever derived from pin_order — never from the position in the list.
func pinnedRail(t *testing.T, library []testsupport.Sticker) []string {
	t.Helper()

	bySlot := map[int]string{}
	for _, one := range library {
		if one.PinOrder == 0 {
			continue
		}
		if existing, taken := bySlot[one.PinOrder]; taken {
			t.Fatalf("slot %d held by both %s and %s", one.PinOrder, existing, one.ID)
		}
		bySlot[one.PinOrder] = one.ID
	}

	rail := make([]string, 0, len(bySlot))
	for slot := 1; slot <= len(bySlot); slot++ {
		id, filled := bySlot[slot]
		if !filled {
			t.Fatalf("slot %d is empty but %d stickers are pinned", slot, len(bySlot))
		}
		rail = append(rail, id)
	}
	return rail
}

// The rail is a choice, so it has to keep the order it was chosen in — not the
// order the stickers were uploaded in, which is what the library is sorted by.
func TestAQuickRailKeepsTheOrderItWasPinnedIn(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	first := app.AddSticker(testsupport.JPEG(t, 100, 100), "1.jpg")
	second := app.AddSticker(testsupport.JPEG(t, 100, 100), "2.jpg")
	third := app.AddSticker(testsupport.JPEG(t, 100, 100), "3.jpg")

	// Deliberately not the upload order, and deliberately not all of them.
	library := app.PinStickers(third.ID, first.ID)

	if rail := pinnedRail(t, library); !slices.Equal(rail, []string{third.ID, first.ID}) {
		t.Fatalf("rail = %v, want %v", rail, []string{third.ID, first.ID})
	}
	// The library keeps its own newest-first order regardless of pinning.
	if library[0].ID != third.ID || library[2].ID != first.ID {
		t.Fatalf("library order changed: %+v", library)
	}
	for _, one := range library {
		if one.ID == second.ID && one.PinOrder != 0 {
			t.Fatalf("unpinned sticker reported slot %d", one.PinOrder)
		}
	}

	// A fresh read agrees with what the write answered.
	if rail := pinnedRail(t, app.ListStickers()); !slices.Equal(rail, []string{third.ID, first.ID}) {
		t.Fatalf("rail after reload = %v, want %v", rail, []string{third.ID, first.ID})
	}
}

// Reordering moves a sticker into a slot another one currently holds. The
// unique index would refuse that if the slots were not cleared first, so this
// is the case that says the write is a replacement rather than an upsert.
func TestAQuickRailCanBeReorderedAndCleared(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	first := app.AddSticker(testsupport.JPEG(t, 100, 100), "1.jpg")
	second := app.AddSticker(testsupport.JPEG(t, 100, 100), "2.jpg")

	app.PinStickers(first.ID, second.ID)

	swapped := app.PinStickers(second.ID, first.ID)
	if rail := pinnedRail(t, swapped); !slices.Equal(rail, []string{second.ID, first.ID}) {
		t.Fatalf("rail after swap = %v, want %v", rail, []string{second.ID, first.ID})
	}

	// Clearing is how someone goes back to the default of "the most recent
	// four" without having to pick four they do not want.
	cleared := app.PinStickers()
	if rail := pinnedRail(t, cleared); len(rail) != 0 {
		t.Fatalf("rail after clearing = %v, want empty", rail)
	}
}

// Deleting a pinned sticker has to give up its slot: a rail pointing at
// something the picker no longer offers would render a gap nobody can fill.
func TestDeletingAPinnedStickerFreesItsSlot(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	pinned := app.AddSticker(testsupport.JPEG(t, 100, 100), "1.jpg")
	other := app.AddSticker(testsupport.JPEG(t, 100, 100), "2.jpg")

	app.PinStickers(pinned.ID)

	if resp := app.DeleteSticker(pinned.ID); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete sticker: status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	library := app.ListStickers()
	if rail := pinnedRail(t, library); len(rail) != 0 {
		t.Fatalf("rail still holds %v after the pinned sticker was deleted", rail)
	}

	// And the freed slot takes a new sticker without any tidying up first.
	if rail := pinnedRail(t, app.PinStickers(other.ID)); !slices.Equal(rail, []string{other.ID}) {
		t.Fatalf("rail = %v, want %v", rail, []string{other.ID})
	}
}

// Both of these are the caller's mistake to fix, so both are answered as bad
// requests rather than left to surface as a database conflict.
func TestAQuickRailRejectsTooManyAndRepeats(t *testing.T) {
	app := testsupport.NewApp(t)
	app.Onboard("mei@example.com", "小美")

	ids := make([]string, 0, 5)
	for range 5 {
		ids = append(ids, app.AddSticker(testsupport.JPEG(t, 100, 100), "s.jpg").ID)
	}

	if resp := app.PutStickerPins(ids...); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pinning five: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	if resp := app.PutStickerPins(ids[0], ids[0]); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("pinning a repeat: status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// A rejected request changes nothing.
	if rail := pinnedRail(t, app.ListStickers()); len(rail) != 0 {
		t.Fatalf("rail = %v after two rejected writes, want empty", rail)
	}
}

// Someone else's sticker is not pinnable, and gets the same answer as an id
// that never existed — the rule the rest of this domain already follows.
func TestPinningSomeoneElsesStickerIsNotFound(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	hers := app.AddSticker(testsupport.JPEG(t, 100, 100), "hers.jpg")

	// Onboarding switches the session, so everything below runs as 阿哲.
	app.Onboard("zhe@example.com", "阿哲")
	mine := app.AddSticker(testsupport.JPEG(t, 100, 100), "mine.jpg")

	if resp := app.PutStickerPins(hers.ID); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pinning someone else's sticker: status = %d, want %d",
			resp.StatusCode, http.StatusNotFound)
	}
	if resp := app.PutStickerPins(uuid.NewString()); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pinning an unknown sticker: status = %d, want %d",
			resp.StatusCode, http.StatusNotFound)
	}

	// The failed writes left the caller's own rail alone.
	if rail := pinnedRail(t, app.ListStickers()); len(rail) != 0 {
		t.Fatalf("rail = %v after two rejected writes, want empty", rail)
	}
	if rail := pinnedRail(t, app.PinStickers(mine.ID)); !slices.Equal(rail, []string{mine.ID}) {
		t.Fatalf("rail = %v, want %v", rail, []string{mine.ID})
	}
}
