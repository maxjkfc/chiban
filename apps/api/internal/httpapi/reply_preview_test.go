package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// Only a text message has anything to quote. Every other kind stores no
// content, so a quote of one has to say what it is quoting or it renders as an
// empty box and the reply reads as an answer to nothing.
func TestAQuoteOfAMessageWithNoTextSaysWhatItIs(t *testing.T) {
	app := testsupport.NewApp(t)

	app.Onboard("mei@example.com", "小美")
	group := app.CreateGroup("午餐團")

	image := app.UploadChatMedia(testsupport.JPEG(t, 120, 120), "lunch.jpg")
	animation := app.UploadChatMedia(testsupport.AnimatedGIF(t, 40, 40, 4), "wave.gif")
	badge := app.AddSticker(testsupport.JPEG(t, 120, 120), "cat.jpg")

	parents := map[string]string{}
	for name, resp := range map[string]*http.Response{
		"image":   app.SendMedia(group.ID, image.ID),
		"gif":     app.SendMedia(group.ID, animation.ID),
		"sticker": app.SendSticker(group.ID, badge.ID),
	} {
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("send %s: status = %d, want %d", name, resp.StatusCode, http.StatusCreated)
		}
		var sent testsupport.Message
		app.DecodeJSON(resp, &sent)
		parents[name] = sent.ID
	}
	// A meal card is created by sharing, not by sending, and it carries no
	// content either — the fourth kind with the same problem.
	meal := app.CreateMeal(testsupport.JPEG(t, 200, 200))
	if resp := app.ShareMeal(meal.ID, group.ID); resp.StatusCode != http.StatusNoContent &&
		resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("share meal: status = %d", resp.StatusCode)
	}
	for _, m := range app.History(group.ID, "").Messages {
		if m.Type == "meal" {
			parents["meal"] = m.ID
		}
	}
	if parents["meal"] == "" {
		t.Fatal("sharing a meal produced no card to reply to")
	}

	for want, parentID := range parents {
		resp := app.Request(http.MethodPost, "/api/v1/groups/"+group.ID+"/messages", map[string]string{
			"content":             "這個好",
			"client_message_id":   uuid.NewString(),
			"reply_to_message_id": parentID,
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("reply to %s: status = %d, want %d", want, resp.StatusCode, http.StatusCreated)
		}

		var reply testsupport.Message
		app.DecodeJSON(resp, &reply)
		if reply.ReplyTo == nil {
			t.Fatalf("the reply to a %s carries no quote", want)
		}
		if reply.ReplyTo.Type != want {
			t.Fatalf("quoting a %s reports type %q", want, reply.ReplyTo.Type)
		}
	}
}
