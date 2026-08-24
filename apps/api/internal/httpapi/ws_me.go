package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/maxjkfc/chiban/apps/api/internal/auth"
)

type clientWsMessage struct {
	Type     string `json:"type"`      // "focus" or "ping"
	GroupID  string `json:"group_id"`  // target group being viewed
	DeviceID string `json:"device_id"` // client device identifier
}

// userEventsSocketHandler handles GET /api/v1/ws/me
// A global WebSocket for in-app events across all user's joined groups with focus lease heartbeat.
func userEventsSocketHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := auth.UserFromContext(r.Context())

		subscription := d.Chat.SubscribeUser(r.Context(), user.ID)
		defer subscription.Close()

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: originPatterns(d.WebOrigin),
		})
		if err != nil {
			d.Logger.Error("global websocket handshake failed", "error", err)
			return
		}
		defer conn.CloseNow()

		ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
		defer cancel()

		var lastDeviceID string

		// Reader goroutine for client heartbeats (focus lease and ping)
		go func() {
			defer cancel()
			for {
				var msg clientWsMessage
				if err := wsjson.Read(ctx, conn, &msg); err != nil {
					if lastDeviceID != "" && d.FocusManager != nil {
						d.FocusManager.ClearLease(user.ID, lastDeviceID)
					}
					return
				}

				switch msg.Type {
				case "focus":
					if msg.DeviceID != "" {
						lastDeviceID = msg.DeviceID
						if gid, err := uuid.Parse(msg.GroupID); err == nil && d.FocusManager != nil {
							// Verify user is actually a member of the group before granting focus
							if allowed, err := d.Chat.MayReceive(ctx, user.ID, gid); err == nil && allowed {
								d.FocusManager.RenewLease(user.ID, msg.DeviceID, gid)
							}
						}
					}
				case "blur":
					if msg.DeviceID != "" && d.FocusManager != nil {
						d.FocusManager.ClearLease(user.ID, msg.DeviceID)
					}
				case "ping":
					writeCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
					_ = wsjson.Write(writeCtx, conn, map[string]string{"type": "pong"})
					cancelPing()
				}
			}
		}()

		// Writer loop for events
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-subscription.Events:
				if !open {
					return
				}

				// Check that user is a member of the event's group
				allowed, err := d.Chat.MayReceive(ctx, user.ID, event.GroupID())
				if err != nil {
					d.Logger.Error("user socket membership check failed", "error", err)
					return
				}
				if !allowed {
					continue // Silently skip events from groups user is not in
				}

				writeCtx, cancelWrite := context.WithTimeout(ctx, 10*time.Second)
				err = wsjson.Write(writeCtx, conn, newSocketEvent(event))
				cancelWrite()
				if err != nil {
					return
				}
			}
		}
	}
}
