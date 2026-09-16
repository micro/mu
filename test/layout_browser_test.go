package test

import (
	"encoding/json"
	"errors"
	"mu/account"
	"mu/home"
	"mu/inbox"
	"mu/internal/api"
	"mu/internal/auth"
	"mu/internal/thread"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Exercise actual command and authentication renderers with isolated data.
// Network access is refused; neither layout nor arrival should run a model.
func TestPageCompositionInBrowser(t *testing.T) {
	if os.Getenv("MU_LAYOUT_BROWSER") == "" && os.Getenv("MU_LAYOUT_EXPORT") == "" {
		t.Skip("set MU_LAYOUT_BROWSER to run browser layout checks")
	}
	old := http.DefaultTransport
	http.DefaultTransport = layoutTransport{}
	defer func() { http.DefaultTransport = old }()
	const who = "layout_reader"
	if err := auth.Create(&auth.Account{ID: who, Name: "Alex Example", Admin: true, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	conversation := thread.Open(who, thread.WebClient, "layout-saved")
	thread.Add(thread.Message{Account: who, Thread: conversation.ID, Role: thread.RolePerson, Text: "My saved question"})
	thread.Add(thread.Message{Account: who, Thread: conversation.ID, Role: thread.RoleAgent, Text: "The saved response."})
	pages := map[string]string{}
	for path, handler := range map[string]http.HandlerFunc{
		"/": home.Index, "/home": home.Handler, "/?session=" + conversation.ID: home.Index,
		"/login": account.Login, "/signup": account.Signup, "/about": home.AboutHandler,
		"/privacy": home.PrivacyHandler, "/pricing": home.PricingHandler, "/contact": home.ContactHandler, "/status": home.StatusHandler,
	} {
		r := httptest.NewRequest("GET", path, nil)
		if path == "/home" || strings.HasPrefix(path, "/?session=") {
			r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		}
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d", path, w.Code)
		}
		pages[path] = w.Body.String()
	}
	oldOps := api.Operations
	api.Operations = inbox.PublicOperations()
	defer func() { api.Operations = oldOps }()
	responses := map[string]json.RawMessage{}
	for _, command := range []string{"help", "inbox", "account", "admin", "admin logs", "brief"} {
		body, _ := json.Marshal(map[string]string{"command": command})
		r := httptest.NewRequest("POST", "/command", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		w := httptest.NewRecorder()
		home.CommandHandler(w, r)
		if w.Code != 200 {
			t.Fatalf("command %s: %d %s", command, w.Code, w.Body.String())
		}
		responses[command] = append([]byte(nil), w.Body.Bytes()...)
	}
	input, _ := json.Marshal(map[string]any{"pages": pages, "responses": responses, "saved": conversation.ID})
	if path := os.Getenv("MU_LAYOUT_EXPORT"); path != "" {
		if err := os.WriteFile(path, input, 0600); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("MU_LAYOUT_BROWSER") == "" {
			return
		}
	}
	cmd := exec.Command("node", "../internal/app/testdata/layout.cjs")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser checks: %v\n%s", err, out)
	}
	t.Log(string(out))
}

type layoutTransport struct{}

func (layoutTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network disabled in layout test")
}
