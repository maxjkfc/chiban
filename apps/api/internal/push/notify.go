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
	NotifyMessage(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, messageType string, content string)
	NotifyMealShare(ctx context.Context, groupIDs []uuid.UUID, senderID uuid.UUID, mealID uuid.UUID)
}

func (s *Service) NotifyMessage(ctx context.Context, groupID uuid.UUID, senderID uuid.UUID, messageType string, content string) {
	body := content
	switch messageType {
	case "image":
		body = "📷 傳送了一張圖片"
	case "gif":
		body = "🎞️ 傳送了一個 GIF"
	case "sticker":
		body = "✨ 傳送了一個貼圖"
	case "meal":
		body = "🍱 分享了一餐"
	}
	if body == "" {
		body = "傳送了一則訊息"
	}

	s.NotifyGroup(ctx, groupID, senderID, Payload{
		Title:   "吃伴",
		Body:    body,
		URL:     fmt.Sprintf("/groups/%s", groupID.String()),
		GroupID: groupID,
		Tag:     groupID.String(),
	})
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
