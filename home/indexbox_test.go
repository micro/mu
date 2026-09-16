package home

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLandingIsRenderedWithoutAClientBootstrap(t *testing.T) {
	w := httptest.NewRecorder()
	Index(w, httptest.NewRequest("GET", "/", nil))
	body := w.Body.String()
	for _, want := range []string{"A personal assistant", `id="mu-chat-input"`, `/mu.css?`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, gone := range []string{`id="root"`, `/client/assets/`} {
		if strings.Contains(body, gone) {
			t.Errorf("retired frontend remains: %s", gone)
		}
	}
}
