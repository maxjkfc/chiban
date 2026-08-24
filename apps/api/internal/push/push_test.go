package push_test

import (
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/push"
)

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{
			name:     "valid apple push endpoint",
			endpoint: "https://web.push.apple.com/QC891238912",
			wantErr:  false,
		},
		{
			name:     "valid fcm google endpoint",
			endpoint: "https://fcm.googleapis.com/fcm/send/12345",
			wantErr:  false,
		},
		{
			name:     "valid mozilla push endpoint",
			endpoint: "https://updates.push.services.mozilla.com/wpush/v2/12345",
			wantErr:  false,
		},
		{
			name:     "valid windows notify endpoint",
			endpoint: "https://db5.notify.windows.com/w/?token=12345",
			wantErr:  false,
		},
		{
			name:     "reject http scheme",
			endpoint: "http://fcm.googleapis.com/fcm/send/12345",
			wantErr:  true,
		},
		{
			name:     "reject internal ip (SSRF attempt)",
			endpoint: "https://192.168.1.1/push",
			wantErr:  true,
		},
		{
			name:     "reject localhost (SSRF attempt)",
			endpoint: "https://localhost:8080/push",
			wantErr:  true,
		},
		{
			name:     "reject unauthorized external host",
			endpoint: "https://attacker.com/evil/push",
			wantErr:  true,
		},
		{
			name:     "reject invalid url format",
			endpoint: "not-a-valid-url",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := push.ValidateEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			}
		})
	}
}
