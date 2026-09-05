package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggedOutRootIsMicro(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()

	Index(w, r)
	body := w.Body.String()

	if !strings.Contains(body, "<title>Micro</title>") {
		t.Fatalf("logged-out root title is not Micro: %q", body)
	}
	if !strings.Contains(body, `<div class="lbrand">Micro</div>`) {
		t.Fatalf("logged-out root wordmark is not Micro: %q", body)
	}
}
