package inbox

import (
	"mu/internal/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSavedCollectionLinksPreserveDestinationAndPostBody(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		r := httptest.NewRequest(method, "/inbox?view=saved&type=file", strings.NewReader("action=file&name=report.txt"))
		w := httptest.NewRecorder()
		collectionView(w, r, &auth.Account{ID: "legacy-library"}, "saved")
		want := 303
		if method == "POST" {
			want = 307
		}
		if w.Code != want || w.Header().Get("Location") != "/home/library?type=file" {
			t.Fatalf("%s: %d %s", method, w.Code, w.Header().Get("Location"))
		}
	}
}
