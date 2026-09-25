package server

import (
	"mu/internal/service"
	"mu/service/mail"
	"mu/service/news"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppIntroductionPreservesResourceAndAPIRoutes(t *testing.T) {
	for _, spec := range []service.Spec{mail.Spec, news.Spec} {
		if _, ok := service.SpecFor(spec.Name); !ok {
			if err := service.Register(spec); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		method, path, accept string
		protected, want      bool
	}{
		{"GET", "/mail", "text/html", true, true},
		{"GET", "/news", "text/html", false, true},
		{"GET", "/news?view=public", "text/html", false, false},
		{"GET", "/mail?id=private", "text/html", true, false},
		{"GET", "/mail", "application/json", true, false},
		{"POST", "/mail", "text/html", true, false},
		{"GET", "/mail/message", "text/html", true, false},
		{"GET", "/api/v1/mail/list", "application/json", true, false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Accept", tc.accept)
		w := httptest.NewRecorder()
		if got := appIntroduction(w, r, tc.protected); got != tc.want {
			t.Fatalf("%s %s: %v", tc.method, tc.path, got)
		}
		if !tc.want {
			if w.Body.Len() != 0 {
				t.Fatal("intercepted a resource/API request")
			}
			continue
		}
		body := w.Body.String()
		if !strings.Contains(body, "Create an account") || !strings.Contains(body, "/login?redirect=%2F") {
			t.Fatal("missing sign-in destination")
		}
		if tc.protected && strings.Contains(body, "?view=public") {
			t.Fatal("private app offered public access")
		}
		if !tc.protected && !strings.Contains(body, "?view=public") {
			t.Fatal("public browsing no longer accessible")
		}
	}
}
