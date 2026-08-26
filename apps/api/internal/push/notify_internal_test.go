package push

import "testing"

// TestMessageNotificationBodyNeverLeaksContent guards the exact bug fixed in
// this change: a push notification body must never echo the message's real
// text, since it renders on a locked screen. Only a fixed, generic string per
// message type is allowed.
func TestMessageNotificationBodyNeverLeaksContent(t *testing.T) {
	secret := "彼此才知道的秘密內容，不該出現在鎖定畫面"

	tests := []struct {
		messageType string
		want        string
	}{
		{messageType: "text", want: "傳送了一則訊息"},
		{messageType: "image", want: "📷 傳送了一張圖片"},
		{messageType: "gif", want: "🎞️ 傳送了一個 GIF"},
		{messageType: "sticker", want: "✨ 傳送了一個貼圖"},
		{messageType: "meal", want: "🍱 分享了一餐"},
		{messageType: "unknown-future-type", want: "傳送了一則訊息"},
	}

	for _, tt := range tests {
		t.Run(tt.messageType, func(t *testing.T) {
			got := messageNotificationBody(tt.messageType)
			if got != tt.want {
				t.Errorf("messageNotificationBody(%q) = %q, want %q", tt.messageType, got, tt.want)
			}
			if got == secret {
				t.Errorf("messageNotificationBody(%q) leaked message content", tt.messageType)
			}
		})
	}
}
