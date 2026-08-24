package httpapi_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

func TestUserWebSocketAndFocusHeartbeat(t *testing.T) {
	app := testsupport.NewApp(t)

	_ = app.RegisterUser("usera@example.com")
	group := app.CreateGroup("Lunch Bunch")
	invite := app.CreateInvite(group.ID)

	// Convert http:// to ws://
	wsURL := strings.Replace(app.BaseURL, "http://", "ws://", 1) + "/api/v1/ws/me"

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Connect userA's global websocket with session cookie
	conn, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: app.Client,
	})
	if err != nil {
		t.Fatalf("dial global ws: %v, resp status: %v", err, resp)
	}
	defer conn.CloseNow()

	// Send Ping, expect Pong
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "ping"}); err != nil {
		t.Fatalf("send ping: %v", err)
	}

	var pongRes map[string]string
	if err := wsjson.Read(ctx, conn, &pongRes); err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if pongRes["type"] != "pong" {
		t.Fatalf("expected pong, got %v", pongRes)
	}

	// Send Focus Heartbeat for group
	focusMsg := map[string]string{
		"type":      "focus",
		"group_id":  group.ID,
		"device_id": "device-mobile-1",
	}
	if err := wsjson.Write(ctx, conn, focusMsg); err != nil {
		t.Fatalf("send focus: %v", err)
	}

	// Register userB and join group
	_ = app.RegisterUser("userb@example.com")
	app.JoinGroup(invite.Code)

	// UserB posts a message to group
	_ = app.SendMessage(group.ID, "Hello from userB")

	// Verify receiving the in-app message event on userA's connection
	var event map[string]any
	readCtx, cancelRead := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelRead()

	if err := wsjson.Read(readCtx, conn, &event); err != nil {
		t.Fatalf("userA expected in-app message event on global ws: %v", err)
	}
	if event["type"] != "message" {
		t.Fatalf("expected event type 'message', got %v (full event: %v)", event["type"], event)
	}
}
