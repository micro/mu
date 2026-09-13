package home

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/places"
	"mu/service/video"
	"mu/service/web"
)

func TestAppsAlwaysOfferUsefulDefaults(t *testing.T) {
	for _, spec := range []service.Spec{news.Spec, markets.Spec, web.Spec, places.Spec, video.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	for _, acc := range []*auth.Account{nil, {}, {Pinned: []string{}}, {Pinned: []string{"removed-service"}}} {
		body := appsHTML(acc)
		for _, name := range []string{"news", "markets", "web", "places"} {
			if !strings.Contains(body, `href="/`+name+`"`) {
				t.Errorf("missing default %s for %#v", name, acc)
			}
		}
		if !strings.Contains(body, `<a class="card-head-link" href="/services">Services</a>`) || strings.Contains(body, "Go to services") {
			t.Error("launcher heading or catalogue link misplaced")
		}
	}
	body := appsHTML(&auth.Account{Pinned: []string{"video"}})
	if strings.Index(body, `href="/video"`) < 0 || strings.Index(body, `href="/video"`) > strings.Index(body, `href="/news"`) {
		t.Error("explicit pins were not placed first")
	}
}
