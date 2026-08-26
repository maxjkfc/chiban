package push

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Notifier defines the interface for triggering push notifications from chat and meal events.
type Notifier interface {
	NotifyMessage(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, messageType string)
	NotifyMealShare(ctx context.Context, groupIDs []uuid.UUID, senderID uuid.UUID, mealID uuid.UUID)
}

func (s *Service) NotifyMessage(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, messageType string) {
	s.NotifyGroup(ctx, groupID, senderID, Payload{
		Title:   "吃伴",
		Body:    messageNotificationBody(messageType),
		URL:     fmt.Sprintf("/groups/%s", groupID.String()),
		GroupID: groupID,
		Tag:     groupID.String(),
	})
}

// messageNotificationBody maps a chat message type to a generic push body.
// It never includes the message's actual content: push notifications only
// announce that something new arrived, so the text stays lock-screen safe.
// The full content is only shown in the in-app toast and the chat itself.
func messageNotificationBody(messageType string) string {
	switch messageType {
	case "image":
		return "📷 傳送了一張圖片"
	case "gif":
		return "🎞️ 傳送了一個 GIF"
	case "sticker":
		return "✨ 傳送了一個貼圖"
	case "meal":
		return "🍱 分享了一餐"
	default:
		return "傳送了一則訊息"
	}
}

// NotifyMealShare handles cross-group meal sharing with recipient-level deduplication.
func (s *Service) NotifyMealShare(ctx context.Context, groupIDs []uuid.UUID, senderID uuid.UUID, mealID uuid.UUID) {
	if s.keys.PublicKey == "" || s.keys.PrivateKey == "" || len(groupIDs) == 0 {
		return
	}

	// Detached context so share completion returns immediately
	detachedCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	go func() {
		defer cancel()

		// Gather subscriptions across all target groups, deduplicating by recipient device_id
		seenDevices := make(map[string]bool)
		var targetSubs []Subscription

		for _, gid := range groupIDs {
			subs, err := s.store.ListByGroupEligible(detachedCtx, gid, senderID)
			if err != nil {
				slog.Error("push: failed to list subs for meal share", "group_id", gid, "err", err)
				continue
			}

			for _, sub := range subs {
				if seenDevices[sub.DeviceID] {
					continue
				}
				if s.focusChecker != nil && s.focusChecker.IsDeviceFocused(sub.UserID, sub.DeviceID, gid) {
					continue
				}
				seenDevices[sub.DeviceID] = true
				targetSubs = append(targetSubs, sub)
			}
		}

		firstGroupID := groupIDs[0]
		payload := Payload{
			Title:   "吃伴",
			Body:    "🍱 分享了一餐",
			URL:     fmt.Sprintf("/groups/%s", firstGroupID.String()),
			GroupID: firstGroupID,
			Tag:     fmt.Sprintf("meal-%s", mealID.String()),
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return
		}

		for _, sub := range targetSubs {
			s.workerSem <- struct{}{}
			go func(sub Subscription) {
				defer func() { <-s.workerSem }()
				s.sendOne(detachedCtx, sub, payloadBytes)
			}(sub)
		}
	}()
}
