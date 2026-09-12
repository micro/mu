package app

import (
	"net/http/httptest"
	"testing"
)

func TestPageScriptCachesWithoutCachingWorkerUpdates(t *testing.T) {
	for _, tc := range []struct{ query, worker, want string }{
		{"", "", "no-cache"}, {Version, "", "public, max-age=86400"}, {Version, "script", "no-cache"}, {"older", "", "no-cache"},
	} {
		r := httptest.NewRequest("GET", "/mu.js?"+tc.query, nil)
		r.Header.Set("Service-Worker", tc.worker)
		w := httptest.NewRecorder()
		Serve().ServeHTTP(w, r)
		if got := w.Header().Get("Cache-Control"); got != tc.want {
			t.Errorf("query %q worker %q: got %q", tc.query, tc.worker, got)
		}
	}
}
