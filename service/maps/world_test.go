package maps

import (
	"net/http/httptest"
	"testing"
)

func TestWorldTileRejectsInvalidCoordinates(t *testing.T) {
	for _, parts := range [][]string{{"0", "0", "0.png"}, {"20", "0", "0.png"}, {"2", "4", "0.png"}, {"2", "0", "-1.png"}, {"bad", "0", "0.png"}} {
		w := httptest.NewRecorder()
		worldTile(w, httptest.NewRequest("GET", "/maps/tiles/world", nil), parts)
		if w.Code != 404 {
			t.Fatalf("%v: status %d", parts, w.Code)
		}
	}
}
