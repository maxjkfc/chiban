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
	// Events is closed when the subscription is closed.
	Events chan Event

	closeOnce sync.Once
}

func (h *Hub) Subscribe(groupID uuid.UUID) *Subscription {
	sub := &Subscription{
		hub:     h,
		groupID: groupID,
		Events:  make(chan Event, subscriberBuffer),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.groups[groupID] == nil {
		h.groups[groupID] = map[*Subscription]struct{}{}
	}
	h.groups[groupID][sub] = struct{}{}
	return sub
}

// Broadcast delivers an event to every live subscription for its group.
func (h *Hub) Broadcast(groupID uuid.UUID, event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for sub := range h.groups[groupID] {
		select {
		case sub.Events <- event:
		default:
			// This connection is too far behind to catch up. Disconnecting it
			// is more honest than skipping the event: the client notices the
			// close, reconnects and refetches, where a silent drop would leave
			// a hole in its history that nothing ever fills. Blocking instead
			// would stall delivery to everyone else.
			h.removeLocked(sub)
		}
	}
}

// Close detaches the subscription. Safe to call more than once.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	s.hub.removeLocked(s)
}

// removeLocked detaches a subscription and closes its channel. The caller must
// hold the hub's mutex, which is why Broadcast can drop a subscription while
// iterating rather than deadlocking against Close.
func (h *Hub) removeLocked(sub *Subscription) {
	if subs := h.groups[sub.groupID]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.groups, sub.groupID)
		}
	}
	sub.closeOnce.Do(func() { close(sub.Events) })
}
