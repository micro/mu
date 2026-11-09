package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"mu/agent"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

// Exercise the real client HTML, stylesheet and command script with a local
// queued-response fixture. No model or external service is called.
func TestFocusedAssistantBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("browser gate runs separately")
	}
	chrome := os.Getenv("CHROME_PATH")
	if chrome == "" {
		t.Skip("Chrome unavailable")
	}
	const owner = "focused-browser-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	chat := thread.Open(owner, thread.WebClient, "focused-browser-chat")
	thread.Name(owner, chat.ID, "Browser conversation")
	var recoveryReads atomic.Int32
	var failRender atomic.Bool
	var slowStarted atomic.Bool
	releaseSlow := make(chan struct{})
	defer func() {
		select {
		case <-releaseSlow:
		default:
			close(releaseSlow)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mu.css", "/mu.js":
			http.ServeFile(w, r, filepath.Join("../internal/app/html", r.URL.Path[1:]))
		case "/slow-started":
			json.NewEncoder(w).Encode(slowStarted.Load())
		case "/session":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"account": owner, "type": "user"})
		case "/mucp":
			if r.Method != "POST" || r.Header.Get("Micro-Protocol") != "mucp/1" || r.Header.Get("Micro-Service") != "assistant" || (r.Header.Get("Micro-Endpoint") != "Assistant.Ask" && r.Header.Get("Micro-Endpoint") != "Assistant.ReadThread") {
				t.Error("client did not use MUCP")
				http.Error(w, "bad request", 400)
				return
			}
			if r.Header.Get("Micro-Endpoint") == "Assistant.ReadThread" {
				if recoveryReads.Add(1) == 1 {
					failRender.Store(true)
					thread.Add(thread.Message{Account: owner, Thread: chat.ID, Role: thread.RoleAgent, Text: "**Recovered** reply"})
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Micro-Protocol", "mucp/1")
				w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
				json.NewEncoder(w).Encode(map[string]any{"pending": false, "messages": []any{}})
				return
			}
			var request map[string]any
			if json.NewDecoder(r.Body).Decode(&request) != nil || request["mode"] != "ACCESS_MODE_PERSONAL" || (request["prompt"] != "Weather please" && request["prompt"] != "Wait please") {
				t.Error("invalid MUCP body")
			}
			if request["prompt"] == "Wait please" {
				slowStarted.Store(true)
				select {
				case <-releaseSlow:
				case <-r.Context().Done():
					return
				}
			}
			thread.Add(thread.Message{Account: owner, Thread: chat.ID, Role: thread.RolePerson, Text: "Weather please"})
			thread.Add(thread.Message{Account: owner, Thread: chat.ID, Role: thread.RoleAgent, Text: "**Forecast**\n\n- Clear skies"})
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Micro-Protocol", "mucp/1")
			w.Header().Set("Micro-Id", r.Header.Get("Micro-Id"))
			json.NewEncoder(w).Encode(map[string]string{"thread": chat.ID, "agentId": "micro", "text": "**Forecast**\n\n- Clear skies"})
		default:
			if failRender.Swap(false) {
				http.Error(w, "temporary render failure", 503)
				return
			}
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
			agent.AssistantHandler(w, r)
		}
	}))
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-dev-shm-usage", true))
	alloc, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for _, width := range []int64{320, 390, 1440} {
		var valid bool
		err := chromedp.Run(ctx,
			emulation.SetDeviceMetricsOverride(width, 844, 1, width < 700),
			chromedp.Navigate(server.URL+"/assistant/micro?new=1"),
			chromedp.WaitVisible("#command-input"),
			chromedp.Evaluate(`document.documentElement.scrollWidth <= innerWidth && !document.querySelector('.runtime-navigation') && document.querySelector('#send').getBoundingClientRect().bottom <= innerHeight && [...document.querySelectorAll('header a')].every(a=>{const r=a.getBoundingClientRect();return r.left>=0 && r.right<=innerWidth})`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("empty client layout at %d", width)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("assistant-empty-%d.png", width)), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
		err = chromedp.Run(ctx,
			chromedp.SendKeys("#command-input", "Weather please"),
			chromedp.Evaluate(`document.querySelector('#command-form').requestSubmit()`, nil),
			chromedp.Poll(`!!document.querySelector('#responses .answer ul')`, nil),
			chromedp.Evaluate(`location.pathname==='/assistant/micro' && document.querySelector('#responses .answer strong').textContent==='Forecast' && document.documentElement.scrollWidth<=innerWidth && document.querySelector('#send').getBoundingClientRect().bottom<=innerHeight`, &valid),
		)
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("populated client at %d", width)
		}
		err = chromedp.Run(ctx, chromedp.Evaluate(`(()=>{
            const status=document.querySelector('[data-voice-status]'), form=document.querySelector('#command-form');
            const top=form.getBoundingClientRect().top;
            for(const text of ['Reading reply…','Microphone access was denied. You can still type.','']) {
                status.textContent=text;
                if(Math.abs(form.getBoundingClientRect().top-top)>0.5)return false;
            }
            return true;
        })()`, &valid))
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("voice feedback shifts composer at %d", width)
		}
		if dir := os.Getenv("UI_SCREENSHOTS"); dir != "" {
			var image []byte
			if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&image)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("assistant-%d.png", width)), image, 0600); err != nil {
				t.Fatal(err)
			}
		}
		err = chromedp.Run(ctx, chromedp.Click(`header a[href="/assistant?view=chats"]`), chromedp.WaitVisible(".collection-list"), chromedp.Evaluate(`document.querySelector('.collection-list').textContent.includes('Browser conversation') && !document.querySelector('.runtime-navigation')`, &valid))
		if err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatal("chat navigation")
		}
	}
	thread.Add(thread.Message{Account: owner, Thread: chat.ID, Role: thread.RolePerson, Text: "A request accepted before disconnect"})
	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/assistant/micro?session="+chat.ID),
		chromedp.Poll(`Array.from(document.querySelectorAll('#responses .answer strong')).some(e=>e.textContent==='Recovered')`, nil),
	); err != nil {
		t.Fatalf("MUCP reconnect recovery: %v", err)
	}

	if err := chromedp.Run(ctx,
		chromedp.Navigate(server.URL+"/assistant/micro?new=1"),
		chromedp.WaitVisible("#command-input"),
		chromedp.Evaluate(`(()=>{const original=window.fetch;window.slowDone=false;window.fetch=async(...args)=>{const response=await original(...args);if(args[0]==='/mucp' && args[1]?.body?.includes('Wait please'))window.slowDone=true;return response;};})()`, nil),
		chromedp.SendKeys("#command-input", "Wait please"),
		chromedp.Evaluate(`document.querySelector('#command-form').requestSubmit()`, nil),
		chromedp.Poll(`fetch('/slow-started').then(r=>r.json())`, nil),
		chromedp.Click(`a[href="/assistant/micro?new=1"]`),
		chromedp.Poll(`document.querySelector('#responses').children.length===0`, nil),
	); err != nil {
		t.Fatalf("navigate during running request: %v", err)
	}
	close(releaseSlow)
	var validAfterNavigation bool
	if err := chromedp.Run(ctx,
		chromedp.Poll(`window.slowDone`, nil),
		chromedp.Evaluate(`document.querySelector('#responses').children.length===0 && location.search==='?new=1'`, &validAfterNavigation),
	); err != nil {
		t.Fatal(err)
	}
	if !validAfterNavigation {
		t.Fatal("late response replaced the new chat")
	}

}
