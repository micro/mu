package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"mu/account"
	"mu/admin"
	"mu/agent"
	"mu/home"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/flag"
	recordnotes "mu/internal/notes"
	"mu/internal/result"
	"mu/internal/service"
	"mu/internal/settings"
	"mu/internal/thread"
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
	"mu/work"
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
	settings.Set("GOOGLE_CLIENT_ID", "layout-client")
	settings.Set("GOOGLE_CLIENT_SECRET", "layout-secret")
	t.Cleanup(func() { settings.Set("GOOGLE_CLIENT_ID", ""); settings.Set("GOOGLE_CLIENT_SECRET", "") })
	const who = "layout_reader"
	if err := auth.Create(&auth.Account{ID: who, Name: "Alex Example", Admin: true, Approved: true}); err != nil {
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
	if _, err := apps.CreateApp(who, "Layout app", "layout-app", "A browser layout fixture", "", "<!doctype html><html><body>Preview</body></html>", "", 0, true); err != nil {
		t.Fatal(err)
	}
	sms.Record(who, "in", "+447700900111", "First SMS message", 1)
	sms.Record(who, "out", "+447700900111", "Latest SMS message", 1)
	smsThread := sms.RecordOn(sms.ChannelWhatsApp, who, "in", "+447700900111", "A separate WhatsApp conversation", 1)
	if err := data.IndexSync("video_layout-video", data.KindVideo, "A layout video", "Video description", map[string]any{"channel": "Publisher"}); err != nil {
		t.Fatal(err)
	}
	recordnotes.Add(who, "A note in the inbox", "Remember to book the appointment\nBring the paperwork\n**Call before leaving**\n\n- Check the time\n- Pack a pen")
	if err := data.IndexSync("layout-news", data.KindNews, "A news article", "Article text", map[string]any{"url": "https://example.com/article", "description": "A short article", "posted_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	webThread := thread.Open(who, thread.WebClient, "layout-conversation")
	thread.Add(thread.Message{Thread: webThread.ID, Account: who, Role: thread.RolePerson, From: who, Text: "Find an Arabic fruits video"})
	thread.Add(thread.Message{Thread: webThread.ID, Account: who, Role: thread.RoleAgent, From: "Micro", Text: "Here is a video to watch and save."})
	for i := 0; i < 36; i++ {
		h := thread.Open(who, thread.WebClient, fmt.Sprintf("history-%d", i))
		thread.Add(thread.Message{Thread: h.ID, Account: who, Role: thread.RolePerson, Text: fmt.Sprintf("Saved conversation %d — plans and questions", i)})
	}
	blog.Load()
	if err := blog.CreatePost("A useful post", "This is a populated post used to check the reading view, comment composer and edit controls.", "Alex Example", who, "", false); err != nil {
		t.Fatal(err)
	}
	postID := blog.PostsByAuthorID(who, "")[0].ID
	flag.Load()
	if err := flag.AdminFlag("post", postID, who); err != nil {
		t.Fatal(err)
	}
	auth.RegisterOAuthClient(who, "Desktop MCP client", []string{"http://localhost:9000/oauth/callback"})
	auth.RegisterOAuthClient("", "Self-registered client", nil)
	mail.Load()
	if err := mail.SendMessage("Sarah", "sarah", "Alex", who, "Tomorrow's appointment", "Please bring the paperwork.", "", "layout-mail"); err != nil {
		t.Fatal(err)
	}
	mailID := mail.ListMessages(who, 1)[0].ID
	focused, _, err := agent.CreateAgent(who, "Research", agent.Hosted, "Research carefully.", "", nil, false)
	if err != nil {
		t.Fatal(err)
	}
	auth.UpdatePresence(who)
	inboxThread := ""
	conversationPages := map[string]http.HandlerFunc{}
	for _, client := range []string{"mail", "chat", "sms", "whatsapp"} {
		th := thread.Open(who, client, "a-long-sender-name-for-mobile@example.com")
		if th == nil {
			t.Fatal("could not create inbox fixture")
		}
		thread.Add(thread.Message{Thread: th.ID, Account: who, From: "sender@example.com", Text: "An inbox conversation on " + client + "\nSecond line\n> Keep this typed line"})
		conversationPages["/inbox?id="+th.ID] = inbox.Handler
		if client == "mail" {
			inboxThread = th.ID
		}
		thread.Add(thread.Message{Thread: th.ID, Account: who, Role: thread.RoleAgent, From: "Micro", Text: "I found the details. Here is the summary."})
		thread.Add(thread.Message{Thread: th.ID, Account: who, From: who, Text: "Thanks, please follow up tomorrow.\nI will be there."})
	}

	chat.Open(chat.PairRoom(who, "friend"), who, "friend")
	pages := map[string]string{}
	pages["/components"] = app.RenderHTML("Shared components", "", `<div class="page-stack">
<section class="card"><h2>Review the brief</h2><p>One action needs your attention.</p><span class="status-badge status-doing">Working</span></section>
<div class="collection-list"><a class="collection-item" href="/notes"><span class="collection-title">Arabic fruit videos</span><span class="collection-preview">Saved ideas for this weekend</span><span class="collection-when">Today</span></a></div>
<div class="mail-thread-item card"><span class="mail-thread-subject">Tomorrow’s appointment</span><div class="mail-thread-meta">Sarah · Today</div><div class="mail-thread-row"><span class="mail-thread-preview">Please bring the paperwork when you arrive.</span><span class="mail-thread-time">10:30</span></div></div>
<article class="thumbnail"><img src="/fixture.svg" alt="Fruit"><h3>Learn Arabic fruit names</h3><p class="info">A short video for children</p><button>Save</button></article>
<table class="data-table"><thead><tr><th>Work</th><th>Status</th><th>Updated</th></tr></thead><tbody><tr><td>Find a video</td><td><span class="status-badge status-done">Done</span></td><td>Today</td></tr></tbody></table>
<div class="notice warn">The result needs your review.</div>
</div>`, &auth.Account{ID: who})
	policies := map[string]string{}
	handlers := map[string]http.HandlerFunc{"/home": home.Handler, "/account": account.Account, "/admin": admin.Handler, "/privacy": home.PrivacyHandler, "/pricing": home.PricingHandler, "/about": home.AboutHandler, "/contact": home.ContactHandler, "/inbox/new": inbox.NewHandler, "/inbox": inbox.Handler, "/inbox?id=" + inboxThread: inbox.Handler, "/archive": archive.Handler, "/blog": blog.Handler, "/bookmarks": bookmarks.Handler, "/browser": browser.Handler, "/contacts": contacts.Handler, "/flights": flights.Handler, "/food": food.Handler, "/hazards": hazards.Handler, "/images": images.Handler, "/mail": mail.Handler, "/maps": maps.Handler, "/notify": notify.Handler, "/places": places.Handler, "/prayer": prayer.Handler, "/recall": recall.Handler, "/routes": routes.Handler, "/shell": shell.Handler, "/sms": sms.Handler, "/sms?view=new": sms.Handler, "/sms?id=" + smsThread.ID: sms.Handler, "/social": social.Handler, "/stream": stream.Handler, "/text": text.Handler, "/transit": transit.Handler, "/users": users.Handler, "/wallet": account.Wallet, "/notes": notes.Handler, "/news": news.Handler, "/news?id=layout-news": news.Handler, "/web": web.Handler, "/weather": weather.PageHandler, "/markets": markets.Handler, "/video": video.Handler, "/video?id=layout-video&autoplay=1": video.Handler, "/signup": account.Signup, "/agent/new": agent.NewAgentHandler, "/agents": agent.RosterHandler, "/token": account.TokenHandler, "/apps/new": apps.Handler, "/apps/layout-app/edit": apps.Handler, "/apps": apps.Handler, "/events": events.Handler, "/files": files.Handler, "/docs": docs.Handler, "/": home.Index, "/agent/micro?new=1": agent.Handler, "/work": work.Handler, "/services": api.ToolsPageHandler, "/tasks": tasks.Handler, "/chat": chat.Handler, "/agent/micro": agent.Handler}
	handlers["/chat?view=rooms"] = chat.Handler
	handlers["/blog?write=true"] = blog.Handler
	handlers["/blog/post?id="+postID] = blog.PostHandler
	handlers["/blog/post?id="+postID+"&edit=true"] = blog.PostHandler
	handlers["/inbox?view=history"] = inbox.Handler
	handlers["/inbox/settings"] = inbox.SettingsHandler
	handlers["/inbox/imap"] = inbox.ImapHandler
	for path, handler := range map[string]http.HandlerFunc{
		"/admin/config": admin.ConfigHandler, "/admin/users": admin.UsersHandler,
		"/admin/alerts": admin.AlertsHandler, "/admin/backup": admin.BackupHandler,
		"/admin/log": admin.LogHandler, "/admin/spam": admin.SpamHandler,
		"/admin/status": admin.StatusHandler, "/admin/traffic": admin.TrafficHandler,
		"/admin/invite": admin.InviteHandler,
	} {
		handlers[path] = handler
	}
	handlers["/admin/server"] = admin.ServerHandler
	handlers["/admin/oauth"] = admin.OAuthHandler
	handlers["/admin/moderate"] = admin.ModerateHandler
	handlers["/login"] = account.Login
	handlers["/mail?id="+mailID] = mail.Handler
	handlers["/agent?id="+focused.ID] = agent.Handler
	for _, path := range []string{"/account", "/account/profile", "/account/billing"} {
		handlers[path] = account.Account
	}
	handlers["/privacy"] = home.PrivacyHandler
	handlers["/pricing"] = home.PricingHandler
	handlers["/@"+who] = inbox.PersonHandler
	handlers["/agent/micro?session="+webThread.ID] = agent.Handler
	for path, handler := range conversationPages {
		handlers[path] = handler
	}
	handlers["/notes?id="+recordnotes.All(who)[0].ID] = notes.Handler
	for path, handler := range handlers {
		t.Log("render", path)
		req := httptest.NewRequest("GET", path, nil)
		if path != "/" && path != "/login" && path != "/signup" && path != "/about" && path != "/privacy" && path != "/pricing" && path != "/contact" {
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
	css := app.Styles()

	input, _ := json.Marshal(map[string]any{"resultHTML": app.Results([]result.Item{{Kind: "article", Title: "Dogecoin ETFs struggled for buyers", Summary: "A clear summary of the article you asked for.", URL: "https://example.com/article"}}), "pages": pages, "policies": policies, "css": string(css)})
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
