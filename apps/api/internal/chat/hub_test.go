package chat_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/chat"
)

// What the hub does with a connection that stops reading cannot be reached
// through the HTTP seam: a real WebSocket client is always draining, and the
// buffer only overflows under a stall no test can arrange from outside. This
// is the same kind of exception as the invite race test — a concurrency rule
// that only exists below the API.

// A stalled connection must be dropped, not silently skipped. Skipping leaves
// the client believing it is up to date while its history quietly loses
// messages; dropping makes it reconnect and refetch.
func TestAStalledSubscriberIsDisconnected(t *testing.T) {
	hub := chat.NewHub()
	groupID := uuid.New()

	stalled := hub.Subscribe(groupID)

	// Nobody reads from stalled, so its buffer fills.
	for range 1000 {
		hub.Broadcast(groupID, chat.Message{GroupID: groupID, Content: "洗版"})
	}

	drained := make(chan struct{})
	go func() {
		for range stalled.Messages {
		}
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("a subscriber that stopped reading is still attached to the hub")
	}
}

// One stalled phone must not cost everyone else their messages, which is the
// whole reason the hub drops rather than blocks.
func TestAStalledSubscriberDoesNotStopTheOthers(t *testing.T) {
	hub := chat.NewHub()
	groupID := uuid.New()

	stalled := hub.Subscribe(groupID)
	defer stalled.Close()
	reading := hub.Subscribe(groupID)
	defer reading.Close()

	// Drain one subscriber as a real connection would.
	received := make(chan chat.Message, 1)
	go func() {
		for message := range reading.Messages {
			select {
			case received <- message:
			default:
			}
		}
	}()

	for range 1000 {
		hub.Broadcast(groupID, chat.Message{GroupID: groupID, Content: "還在嗎"})
	}

	select {
	case message := <-received:
		if message.Content != "還在嗎" {
			t.Fatalf("received %q, want %q", message.Content, "還在嗎")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a stalled subscriber stopped delivery to a healthy one")
	}
}
