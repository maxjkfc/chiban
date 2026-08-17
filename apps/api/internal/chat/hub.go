package chat

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBuffer is how many messages a slow connection may fall behind
// before it is dropped. Dropping is deliberate: one stalled phone must not
// hold up the broadcast to everyone else, and a reconnect refetches history.
const subscriberBuffer = 32

// Hub fans messages out to the connections watching each group.
//
// V0.1 runs on one node, so this is a map and a mutex rather than Redis or a
// message queue — a broadcast only has to reach connections held by this
// process. Anything durable is already in PostgreSQL by the time it gets here.
type Hub struct {
	mu     sync.Mutex
	groups map[uuid.UUID]map[*Subscription]struct{}
}

func NewHub() *Hub {
	return &Hub{groups: map[uuid.UUID]map[*Subscription]struct{}{}}
}

// Subscription is one connection's view of a group.
type Subscription struct {
	hub     *Hub
	groupID uuid.UUID
	// Messages is closed when the subscription is closed.
	Messages chan Message

	closeOnce sync.Once
}

func (h *Hub) Subscribe(groupID uuid.UUID) *Subscription {
	sub := &Subscription{
		hub:      h,
		groupID:  groupID,
		Messages: make(chan Message, subscriberBuffer),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.groups[groupID] == nil {
		h.groups[groupID] = map[*Subscription]struct{}{}
	}
	h.groups[groupID][sub] = struct{}{}
	return sub
}

// Broadcast delivers a message to every live subscription for its group.
func (h *Hub) Broadcast(groupID uuid.UUID, message Message) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sub := range h.groups[groupID] {
		select {
		case sub.Messages <- message:
		default:
			// The connection is not keeping up. Dropping the message is
			// correct here: history is authoritative and a reconnect refetches
			// it, whereas blocking would stall every other member's delivery.
		}
	}
}

// Close detaches the subscription. Safe to call more than once.
func (s *Subscription) Close() {
	s.closeOnce.Do(func() {
		s.hub.mu.Lock()
		defer s.hub.mu.Unlock()

		if subs := s.hub.groups[s.groupID]; subs != nil {
			delete(subs, s)
			if len(subs) == 0 {
				delete(s.hub.groups, s.groupID)
			}
		}
		close(s.Messages)
	})
}
