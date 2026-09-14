package apps

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSDKAssetsShipInsideTheBinary(t *testing.T) {
	for _, asset := range []struct{ path, mime, marker string }{
		{"static/sdk.css", "text/css", ".mu-"},
		{"static/sdk.js", "application/javascript", "window.app"},
	} {
		w := httptest.NewRecorder()
		handleStaticFile(w, asset.path, asset.mime)
		if w.Code != 200 || w.Header().Get("Content-Type") != asset.mime || !strings.Contains(w.Body.String(), asset.marker) {
			t.Fatalf("missing embedded asset %s: %d", asset.path, w.Code)
		}
	}
}
