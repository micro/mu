package test

import (
	"encoding/json"
	"fmt"
	"mu/account"
	"mu/admin"
	"mu/home"
	"mu/inbox"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/apps"
	"mu/service/events"
	"mu/service/mail"
	webclient "mu/web"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Exercise the embedded build against real authenticated handlers. No model,
// outbound mail or external provider is called by this test.
func TestReactClientInBrowser(t *testing.T) {
	if os.Getenv("MU_LAYOUT_BROWSER") == "" {
		t.Skip("set MU_LAYOUT_BROWSER to run browser checks")
	}
	const owner = "react_layout"
	if err := auth.Create(&auth.Account{ID: owner, Name: "Alex Example", Admin: true, Approved: true, Zone: "Europe/London"}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		id := fmt.Sprintf("react_user_%02d", i)
		if err := auth.Create(&auth.Account{ID: id, Name: "A long display name for a small mobile screen", Created: time.Now().Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := events.ScheduleBrief(owner, "06:00", "Europe/London", "daily", "morning", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 28; i++ {
		th := thread.Open(owner, "mail", fmt.Sprint("react-mail", i))
		thread.Name(owner, th.ID, fmt.Sprint("Message ", i))
		thread.Add(thread.Message{Account: owner, Thread: th.ID, From: "long-sender-address@example.com", Text: "A compact preview of the message you need to read."})
	}
	th := thread.Open(owner, "mail", "react-reader")
	thread.Name(owner, th.ID, "Your morning brief")
	for i := 0; i < 105; i++ {
		thread.Add(thread.Message{Account: owner, Thread: th.ID, From: "writer@example.com", Text: fmt.Sprint("Recorded message ", i)})
	}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, Role: thread.RoleAgent, Text: "## Your day\n\nRead your brief, then decide what needs doing."})
	other := thread.Open("another-owner", "mail", "private")
	thread.Add(thread.Message{Account: "another-owner", Thread: other.ID, Text: "Must not be visible"})
	thread.Hold("another-owner", other.ID)
	request := thread.Open(owner, thread.SMSClient, "+447700900991")
	thread.Name(owner, request.ID, "Waiting SMS")
	thread.Add(thread.Message{Account: owner, Thread: request.ID, From: "+447700900991", Text: "Please let me in."})
	thread.Hold(owner, request.ID)
	blocked := thread.Open(owner, thread.SMSClient, "+447700900992")
	thread.Name(owner, blocked.ID, "Unwanted SMS")
	thread.Add(thread.Message{Account: owner, Thread: blocked.ID, From: "+447700900992", Text: "An unsolicited message."})
	thread.Hold(owner, blocked.ID)
	conversation := thread.Open(owner, thread.WebClient, "react-dialogue")
	thread.Add(thread.Message{Account: owner, Thread: conversation.ID, Role: thread.RolePerson, Text: "My saved question"})
	thread.Add(thread.Message{Account: owner, Thread: conversation.ID, Role: thread.RoleAgent, Text: "Your earlier conversation is still here."})
	if _, err := apps.CreateApp(owner, "Fruit cards", "react-fruit-cards", "A useful saved app with a longer description that wraps on a phone.", "learning", "<h1>Fruit cards</h1>", "", 0, true); err != nil {
		t.Fatal(err)
	}
	// Rich MIME content still uses the mail decoder and stays sandboxed.
	ref := "<react-report@example.com>"
	if err := mail.SendMessageTo(mail.Delivery{From: "writer@example.com", FromID: "writer@example.com", To: owner, ToID: owner, Subject: "A report", Body: "<h2>A report</h2><table><tr><td>Reading</td><td>Value</td></tr></table>", MessageID: ref}); err != nil {
		t.Fatal(err)
	}
	thread.Add(thread.Message{Account: owner, Thread: th.ID, From: "writer@example.com", Text: "Report content", Ref: ref})
	mux := http.NewServeMux()
	mux.HandleFunc("/", home.Index)
	mux.HandleFunc("/client/assets/", webclient.Assets)
	mux.HandleFunc("/client/state", account.ClientStateHandler)
	mux.HandleFunc("/about", home.AboutHandler)
	mux.HandleFunc("/privacy", home.PrivacyHandler)
	mux.HandleFunc("/pricing", home.PricingHandler)
	mux.HandleFunc("/contact", home.ContactHandler)
	mux.HandleFunc("/status", home.StatusHandler)
	mux.HandleFunc("/inbox", inbox.Handler)
	mux.HandleFunc("/inbox/settings", inbox.SettingsHandler)
	mux.HandleFunc("/inbox/new", inbox.NewHandler)
	mux.HandleFunc("/inbox/delete", inbox.DeleteHandler)
	mux.HandleFunc("/inbox/held", inbox.HeldHandler)
	mux.HandleFunc("/inbox/unread", inbox.UnreadHandler)
	mux.HandleFunc("/admin/users", admin.UsersHandler)
	mux.HandleFunc("/account", account.Account)
	mux.HandleFunc("/account/profile", account.Account)
	mux.HandleFunc("/account/billing", account.Account)
	mux.HandleFunc("/account/place", account.PlaceHandler)
	mux.HandleFunc("/apps", apps.Handler)
	mux.HandleFunc("/mu.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Write([]byte("// No push in fixture"))
	})
	server := httptest.NewServer(webclient.WithData(mux))
	defer server.Close()
	fixture, _ := json.Marshal(map[string]any{"base": server.URL, "session": session.Token, "thread": th.ID, "other": other.ID, "request": request.ID, "blocked": blocked.ID})
	cmd := exec.Command("node", "../web/test/browser.cjs")
	cmd.Stdin = strings.NewReader(string(fixture))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("React browser checks: %v\n%s", err, out)
	}
	t.Log(string(out))
}
