package server

import (
	"net/http/httptest"
	"testing"
)

func TestInstallScriptAlias(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "POST"} {
		w := httptest.NewRecorder()
		installScriptHandler(w, httptest.NewRequest(method, "/install.sh", nil))
		if method == "POST" {
			if w.Code != 405 {
				t.Fatal(w.Code)
			}
		} else if w.Code != 307 || w.Header().Get("Location") != "https://raw.githubusercontent.com/micro/mu/main/install.sh" {
			t.Fatal(w.Code, w.Header())
		}
	}
}
