package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"mu/agent"
	"mu/internal/api"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/group"
	"mu/service/chat"
	host402 "mu/x402"
)

func TestGroupChatBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser flow runs separately")
	}
	browser := os.Getenv("CHROME_PATH")
	if browser == "" {
		t.Skip("Chrome not configured")
	}
	const owner = "browser_group_owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "session", Value: session.Token})
		w, done := app.TimeRequest(w, r)
		defer done()
		switch r.URL.Path {
		case "/mu.js", "/mu.css":
			http.ServeFile(w, r, filepath.Join("../app/html", filepath.Base(r.URL.Path)))
		case "/session":
			host402.SessionHandler(w, api.CredentialRequest(r))
		case "/chat":
			chat.Handler(w, r)
		case "/agent":
			agent.Handler(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	// This fixture is plain HTTP; do not configure a production HTTPS origin.
	for _, key := range []string{"MU_DOMAIN", "PUBLIC_URL", "APP_URL"} {
		t.Setenv(key, "localhost")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	capture := func(name string) {
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name+".png"), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, width := range []int64{390, 1440} {
		if err := chromedp.Run(ctx, emulation.SetDeviceMetricsOverride(width, 900, 1, width < 700), chromedp.Navigate(server.URL+"/agent?view=new-chat&group=new"), chromedp.WaitVisible(`input[name="name"]`), chromedp.SendKeys(`input[name="name"]`, "Family plans")); err != nil {
			t.Fatal(err)
		}
		capture(fmt.Sprintf("group-create-%d", width))
		if err := chromedp.Run(ctx, chromedp.Click(`form[method="post"] button`), chromedp.WaitVisible(`#chat-form`), chromedp.Poll(`document.querySelector('#chat-form button').disabled === false`, nil)); err != nil {
			t.Fatal(err)
		}
		var target string
		if err := chromedp.Run(ctx, chromedp.Evaluate(`new URL(location.href).searchParams.get('id')`, &target)); err != nil {
			t.Fatal(err)
		}
		defer group.Change(group.RoomGroup(target), owner, "delete", "", "")
		if err := chromedp.Run(ctx, chromedp.SendKeys(`#chat-form [name="prompt"]`, "Let's meet on Saturday"), chromedp.Click(`#chat-form button`), chromedp.Poll(`document.querySelector('#chat-form [name="prompt"]').value === '' && document.querySelector('#messages').textContent.includes('Saturday')`, nil)); err != nil {
			t.Fatal(err)
		}
		// A dropped connection replays history but must not duplicate rendered messages.
		if err := chromedp.Run(ctx, chromedp.Evaluate(`roomWs.close()`, nil), chromedp.Poll(`roomWs.readyState === WebSocket.OPEN && document.querySelector('#chat-form button').disabled === false`, nil), chromedp.Poll(`document.querySelectorAll('#messages .message:not(.message-system)').length === 1`, nil)); err != nil {
			t.Fatal(err)
		}
		var fits bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.scrollWidth <= innerWidth`, &fits)); err != nil || !fits {
			t.Fatal("chat overflow", width, err)
		}
		capture(fmt.Sprintf("group-chat-%d", width))
	}
}
