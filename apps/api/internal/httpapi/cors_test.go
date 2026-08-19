package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A browser preflights anything that is not a simple request, so a method the
// router serves but this header omits is a route the web app can never reach.
// The failure is invisible from the API side — the request is refused before it
// arrives — which is why it is pinned here rather than left to a manual check.
func TestPreflightAllowsEveryMethodTheRouterServes(t *testing.T) {
	const origin = "http://localhost:3000"

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/me/sticker-pins", nil)
	request.Header.Set("Origin", origin)
	recorder := httptest.NewRecorder()

	withCORS(origin, http.NotFoundHandler()).ServeHTTP(recorder, request)

	allowed := recorder.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions,
	} {
		if !strings.Contains(allowed, method) {
			t.Errorf("Access-Control-Allow-Methods = %q, missing %s", allowed, method)
		}
	}
}
