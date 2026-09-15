package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTheFrontPageUsesTheEmbeddedClient(t *testing.T) {
	w := httptest.NewRecorder()
	Index(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	if !strings.Contains(body, `id="root"`) || !strings.Contains(body, "/client/assets/") {
		t.Fatal("embedded React client missing")
	}
	for _, old := range []string{"mu.css", "mu-chat-form", "<script>"} {
		if strings.Contains(body, old) {
			t.Errorf("legacy renderer still loaded: %s", old)
		}
	}
}
