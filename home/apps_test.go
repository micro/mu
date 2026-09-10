package home

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/mail"
	"mu/service/news"
	"mu/service/video"
	"mu/service/web"
)

func TestAppsAlwaysOfferUsefulDefaults(t *testing.T) {
	for _, spec := range []service.Spec{news.Spec, video.Spec, web.Spec, mail.Spec} {
		if err := service.Register(spec); err != nil {
			t.Fatal(err)
		}
	}
	for _, acc := range []*auth.Account{nil, {}, {Pinned: []string{}}, {Pinned: []string{"removed-service"}}} {
		body := appsHTML(acc)
		for _, name := range []string{"news", "video", "web", "mail"} {
			if !strings.Contains(body, `href="/`+name+`"`) {
				t.Errorf("missing default %s for %#v", name, acc)
			}
		}
		if !strings.Contains(body, sectionRule("Services")) || !strings.Contains(body, `</div><a href="/services" class="link">Go to services →</a>`) {
			t.Error("launcher heading or catalogue link misplaced")
		}
	}
	body := appsHTML(&auth.Account{Pinned: []string{"video"}})
	if !strings.Contains(body, `href="/video"`) || strings.Contains(body, `href="/news"`) {
		t.Error("explicit pins were not used")
	}
}
