package push_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/maxjkfc/chiban/apps/api/internal/push"
)

func TestFocusManager(t *testing.T) {
	fm := push.NewFocusManager()

	userA := uuid.New()
	userB := uuid.New()
	group1 := uuid.New()
	group2 := uuid.New()
	dev1 := "phone-1"
	dev2 := "tablet-1"

	// Initially not focused
	if fm.IsDeviceFocused(userA, dev1, group1) {
		t.Fatalf("expected device not to be focused initially")
	}

	// Renew lease for userA dev1 in group1
	fm.RenewLease(userA, dev1, group1)
	if !fm.IsDeviceFocused(userA, dev1, group1) {
		t.Fatalf("expected device to be focused in group1")
	}

	// Should not be focused in group2
	if fm.IsDeviceFocused(userA, dev1, group2) {
		t.Fatalf("expected device not to be focused in group2")
	}

	// UserB with same deviceID should NOT be focused (user isolation / spoof protection)
	if fm.IsDeviceFocused(userB, dev1, group1) {
		t.Fatalf("userB should not inherit userA's focus lease")
	}

	// UserA's second device dev2 should NOT be focused
	if fm.IsDeviceFocused(userA, dev2, group1) {
		t.Fatalf("userA dev2 should not be focused")
	}

	// Clear lease
	fm.ClearLease(userA, dev1)
	if fm.IsDeviceFocused(userA, dev1, group1) {
		t.Fatalf("expected device to not be focused after clear")
	}
}
