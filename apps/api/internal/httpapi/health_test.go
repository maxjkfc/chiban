package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/maxjkfc/chiban/apps/api/internal/testsupport"
)

// The API is only useful if it can reach the database, so /healthz reports the
// database round-trip rather than just that the process is alive.
func TestHealthzReportsOKWhenDatabaseIsReachable(t *testing.T) {
	app := testsupport.NewApp(t)

	resp := app.Request(http.MethodGet, "/healthz", nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	app.DecodeJSON(resp, &body)
	if body.Status != "ok" {
		t.Fatalf("status = %q, want %q", body.Status, "ok")
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	app := testsupport.NewApp(t)

	resp := app.Request(http.MethodGet, "/nope", nil)

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
