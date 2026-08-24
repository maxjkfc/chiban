package push

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type leaseKey struct {
	userID   uuid.UUID
	deviceID string
}

type leaseVal struct {
	groupID   uuid.UUID
	expiresAt time.Time
}

// FocusManager tracks ephemeral chat room focus leases for connected client devices.
// A client sends { "type": "focus", "group_id": "...", "device_id": "..." } periodically.
// Focus is leased for 45s and automatically expires if heartbeats stop.
type FocusManager struct {
	mu     sync.RWMutex
	leases map[leaseKey]leaseVal
}

func NewFocusManager() *FocusManager {
	return &FocusManager{
		leases: make(map[leaseKey]leaseVal),
	}
}

// RenewLease sets or updates the focus lease for a authenticated user's device.
func (m *FocusManager) RenewLease(userID uuid.UUID, deviceID string, groupID uuid.UUID) {
	if userID == uuid.Nil || deviceID == "" || groupID == uuid.Nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.leases[leaseKey{userID: userID, deviceID: deviceID}] = leaseVal{
		groupID:   groupID,
		expiresAt: time.Now().Add(45 * time.Second),
	}
}

// ClearLease removes the focus lease for a device (e.g. when navigating away from chat).
func (m *FocusManager) ClearLease(userID uuid.UUID, deviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.leases, leaseKey{userID: userID, deviceID: deviceID})
}

// IsDeviceFocused returns true if the device currently holds an active, non-expired focus lease for groupID.
func (m *FocusManager) IsDeviceFocused(userID uuid.UUID, deviceID string, groupID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, exists := m.leases[leaseKey{userID: userID, deviceID: deviceID}]
	if !exists {
		return false
	}
	if val.groupID != groupID {
		return false
	}
	return time.Now().Before(val.expiresAt)
}
