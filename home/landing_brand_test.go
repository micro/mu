package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggedOutRootIsHome(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()

	Index(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "<title>Micro</title>") {
		t.Fatalf("logged-out root title is not Home | Micro: %q", body)
	}
	if !strings.Contains(body, `class="brand"`) {
		t.Fatalf("logged-out root wordmark is not Micro: %q", body)
	}
}
