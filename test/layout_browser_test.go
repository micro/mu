package test

import (
	"encoding/json"
	"errors"
	"mu/account"
	"mu/agent"
	"mu/home"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/archive"
	"mu/service/blog"
	"mu/service/bookmarks"
	"mu/service/browser"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/flights"
	"mu/service/food"
	"mu/service/hazards"
	"mu/service/images"
	"mu/service/mail"
	"mu/service/maps"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/notes"
	"mu/service/notify"
	"mu/service/places"
	"mu/service/prayer"
	"mu/service/recall"
	"mu/service/routes"
	"mu/service/shell"
	"mu/service/sms"
	"mu/service/social"
	"mu/service/stream"
	"mu/service/tasks"
	"mu/service/text"
	"mu/service/transit"
	"mu/service/users"
	"mu/service/video"
	"mu/service/weather"
	"mu/service/web"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Run with MU_LAYOUT_BROWSER pointing at Chromium and MU_PLAYWRIGHT_MODULE at
// playwright's module. Uses actual handlers, isolated accounts and no providers.
func TestPageCompositionInBrowser(t *testing.T) {
	if os.Getenv("MU_LAYOUT_BROWSER") == "" {
		t.Skip("set MU_LAYOUT_BROWSER to run browser layout checks")
	}
	// Render without contacting providers, including pages that fetch on GET.
	oldTransport := http.DefaultTransport
	http.DefaultTransport = layoutTransport{}
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	t.Setenv("AVIATIONSTACK_API_KEY", "layout-fixture")
	t.Setenv("TWILIO_ACCOUNT_SID", "AC00000000000000000000000000000000")
	t.Setenv("TWILIO_AUTH_TOKEN", "00000000000000000000000000000000")
	t.Setenv("TWILIO_FROM", "+447700900123")
	const who = "layout_reader"
	if err := auth.Create(&auth.Account{ID: who, Admin: true, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	for _, sp := range []service.Spec{news.Spec, weather.Spec, markets.Spec, video.Spec, files.Spec, docs.Spec, events.Spec} {
		if err := service.Register(sp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tasks.Create(who, "Review the brief", "A task to verify action buttons", tasks.Me, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := apps.CreateApp(who, "Layout app", "layout-app", "A browser layout fixture", "", "<!doctype html><html><body>Preview</body></html>", "", 0, false); err != nil {
		t.Fatal(err)
	}
	sms.Record(who, "in", "+447700900111", "First SMS message", 1)
	sms.Record(who, "out", "+447700900111", "Latest SMS message", 1)
	smsThread := sms.RecordOn(sms.ChannelWhatsApp, who, "in", "+447700900111", "A separate WhatsApp conversation", 1)
	pages := map[string]string{}
	policies := map[string]string{}
	for path, handler := range map[string]http.HandlerFunc{"/archive": archive.Handler, "/blog": blog.Handler, "/bookmarks": bookmarks.Handler, "/browser": browser.Handler, "/contacts": contacts.Handler, "/flights": flights.Handler, "/food": food.Handler, "/hazards": hazards.Handler, "/images": images.Handler, "/mail": mail.Handler, "/maps": maps.Handler, "/notify": notify.Handler, "/places": places.Handler, "/prayer": prayer.Handler, "/recall": recall.Handler, "/routes": routes.Handler, "/shell": shell.Handler, "/sms": sms.Handler, "/sms?view=new": sms.Handler, "/sms?id=" + smsThread.ID: sms.Handler, "/social": social.Handler, "/stream": stream.Handler, "/text": text.Handler, "/transit": transit.Handler, "/users": users.Handler, "/wallet": account.Wallet, "/notes": notes.Handler, "/news": news.Handler, "/web": web.Handler, "/weather": weather.PageHandler, "/markets": markets.Handler, "/video": video.Handler, "/video?id=layout-video&autoplay=1": video.Handler, "/signup": account.Signup, "/agent/new": agent.NewAgentHandler, "/agents": agent.RosterHandler, "/token": account.TokenHandler, "/apps/new": apps.Handler, "/apps/layout-app/edit": apps.Handler, "/apps": apps.Handler, "/events": events.Handler, "/files": files.Handler, "/docs": docs.Handler, "/": home.Index, "/home": home.Handler, "/tasks": tasks.Handler, "/chat": chat.Handler, "/agent/micro": agent.Handler} {
		t.Log("render", path)
		req := httptest.NewRequest("GET", path, nil)
		if path != "/" {
			req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		rec := httptest.NewRecorder()
		if strings.HasPrefix(path, "/video?id=") {
			rec.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; frame-src https://www.youtube.com")
		}
		handler(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		pages[path] = rec.Body.String()
		policies[path] = rec.Header().Get("Content-Security-Policy")
	}
	for name := range keptItsPage {
		if _, ok := pages["/"+name]; !ok {
			t.Errorf("service %s has no browser fixture", name)
		}
	}
	css, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	composition, err := os.ReadFile("../internal/app/html/composition.css")
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"pages": pages, "policies": policies, "css": string(css), "composition": string(composition)})
	cmd := exec.Command("node", "../internal/app/testdata/layout.cjs")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser checks: %v\n%s", err, out)
	}
	t.Log(string(out))
}

// Provider data is irrelevant to form geometry; failures are immediate and offline.
type layoutTransport struct{}

func (layoutTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("offline layout fixture")
}
