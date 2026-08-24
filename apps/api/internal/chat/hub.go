package chat

import (
	"sync"

	"github.com/google/uuid"
)

// subscriberBuffer is how many messages a slow connection may fall behind
// before it is dropped. Dropping is deliberate: one stalled phone must not
// hold up the broadcast to everyone else, and a reconnect refetches history.
const subscriberBuffer = 32

// Hub fans messages out to the connections watching each group, as well as
// global user connections listening across their joined groups.
type Hub struct {
	mu     sync.Mutex
	groups map[uuid.UUID]map[*Subscription]struct{}
	users  map[uuid.UUID]map[*UserSubscription]struct{}
}

func NewHub() *Hub {
	return &Hub{
		groups: map[uuid.UUID]map[*Subscription]struct{}{},
		users:  map[uuid.UUID]map[*UserSubscription]struct{}{},
	}
}

// Subscription is one connection's view of a group.
type Subscription struct {
	hub     *Hub
	groupID uuid.UUID
	Events  chan Event

	closeOnce sync.Once
}

// UserSubscription is one connection's global view across all their joined groups.
type UserSubscription struct {
	hub    *Hub
	userID uuid.UUID
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

func (h *Hub) SubscribeUser(userID uuid.UUID) *UserSubscription {
	sub := &UserSubscription{
		hub:    h,
		userID: userID,
		Events: make(chan Event, subscriberBuffer),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.users[userID] == nil {
		h.users[userID] = map[*UserSubscription]struct{}{}
	}
	h.users[userID][sub] = struct{}{}
	return sub
}

// Broadcast delivers an event to every live subscription for its group,
// and to all global user connections currently subscribed.
func (h *Hub) Broadcast(groupID uuid.UUID, event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Deliver to group-specific subscriptions
	for sub := range h.groups[groupID] {
		select {
		case sub.Events <- event:
		default:
			h.removeLocked(sub)
		}
	}

	// Deliver to global user connections
	for _, userSubs := range h.users {
		for sub := range userSubs {
			select {
			case sub.Events <- event:
			default:
				h.removeUserLocked(sub)
			}
		}
	}
}

// Close detaches the subscription. Safe to call more than once.
func (s *Subscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	s.hub.removeLocked(s)
}

func (s *UserSubscription) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()

	s.hub.removeUserLocked(s)
}

func (h *Hub) removeLocked(sub *Subscription) {
	if subs := h.groups[sub.groupID]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.groups, sub.groupID)
		}
	}
	sub.closeOnce.Do(func() { close(sub.Events) })
}

func (h *Hub) removeUserLocked(sub *UserSubscription) {
	if subs := h.users[sub.userID]; subs != nil {
		delete(subs, sub)
		if len(subs) == 0 {
			delete(h.users, sub.userID)
		}
	}
	sub.closeOnce.Do(func() { close(sub.Events) })
}
