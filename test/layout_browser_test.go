package test

import (
	"encoding/json"
	"mu/account"
	"mu/agent"
	"mu/home"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/service/apps"
	"mu/service/chat"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/markets"
	"mu/service/news"
	"mu/service/tasks"
	"mu/service/video"
	"mu/service/weather"
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
	pages := map[string]string{}
	for path, handler := range map[string]http.HandlerFunc{"/agent/new": agent.NewAgentHandler, "/agents": agent.RosterHandler, "/token": account.TokenHandler, "/apps/new": apps.Handler, "/events": events.Handler, "/files": files.Handler, "/docs": docs.Handler, "/": home.Index, "/home": home.Handler, "/tasks": tasks.Handler, "/chat": chat.Handler, "/agent/micro": agent.Handler} {
		req := httptest.NewRequest("GET", path, nil)
		if path != "/" {
			req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		rec := httptest.NewRecorder()
		handler(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		pages[path] = rec.Body.String()
	}
	css, err := os.ReadFile("../internal/app/html/mu.css")
	if err != nil {
		t.Fatal(err)
	}
	composition, err := os.ReadFile("../internal/app/html/composition.css")
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"pages": pages, "css": string(css), "composition": string(composition)})
	cmd := exec.Command("node", "../internal/app/testdata/layout.cjs")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser checks: %v\n%s", err, out)
	}
	t.Log(string(out))
}
