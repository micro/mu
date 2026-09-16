package test

import (
	"encoding/json"
	"mu/account"
	"mu/admin"
	"mu/agent"
	firstparty "mu/app"
	"mu/home"
	"mu/internal/api"
	legacyapp "mu/internal/app"
	"mu/internal/auth"
	"mu/internal/service"
	"mu/internal/sshaccess"
	"mu/internal/thread"
	"mu/internal/tool"
	"mu/service/apps"
	"mu/service/bookmarks"
	"mu/service/chat"
	"mu/service/contacts"
	"mu/service/docs"
	"mu/service/events"
	"mu/service/files"
	"mu/service/mail"
	"mu/service/notes"
	"mu/service/tasks"
	webclient "mu/web"
	"mu/work"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestReactApplicationsInBrowser(t *testing.T) {
	if os.Getenv("MU_LAYOUT_BROWSER") == "" {
		t.Skip("set MU_LAYOUT_BROWSER to run browser checks")
	}
	const owner = "react_apps"
	if err := auth.Create(&auth.Account{ID: owner, Name: "App Reader", Admin: true, Approved: true}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []service.Spec{docs.Spec, notes.Spec, contacts.Spec, events.Spec, files.Spec, tasks.Spec} {
		if err := service.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	tool.Load(service.Specs())
	if err := auth.Create(&auth.Account{ID: "react_sender", Name: "Sender", Approved: true}); err != nil {
		t.Fatal(err)
	}
	if err := mail.SendMessage("Sender", "react_sender", "App Reader", owner, "Migration mail", "A preserved message body.", "", ""); err != nil {
		t.Fatal(err)
	}
	older := thread.Open(owner, thread.WebClient, "older-react-app")
	thread.Add(thread.Message{Thread: older.ID, Account: owner, Role: thread.RolePerson, Text: "Older selected question"})
	newer := thread.Open(owner, thread.WebClient, "newer-react-app")
	thread.Add(thread.Message{Thread: newer.ID, Account: owner, Role: thread.RolePerson, Text: "Newer saved question"})
	mux := http.NewServeMux()
	icons := map[string]bool{}
	for _, entry := range firstparty.Entries() {
		if entry.Icon != "" && !icons[entry.Icon] {
			mux.Handle("/"+entry.Icon, legacyapp.Serve())
			icons[entry.Icon] = true
		}
	}
	mux.HandleFunc("/agent/new", firstparty.Page(agent.NewAgentHandler, "New agent"))
	mux.HandleFunc("/agents", firstparty.Page(agent.AgentsHandler, "Agents"))
	mux.HandleFunc("/chat", firstparty.Page(chat.Handler, "Chat"))
	mux.HandleFunc("/agent/", agent.Handler)
	mux.HandleFunc("/", home.Index)
	mux.HandleFunc("/about", home.AboutHandler)
	mux.HandleFunc("/privacy", home.PrivacyHandler)
	mux.HandleFunc("/client/assets/", webclient.Assets)
	mux.Handle("/dm-sans-latin.woff2", legacyapp.Serve())
	mux.Handle("/geist-mono-latin.woff2", legacyapp.Serve())
	mux.HandleFunc("/client/ssh", sshaccess.ClientHandler)
	mux.HandleFunc("/mail", firstparty.Page(mail.Handler, "Mail"))
	mux.HandleFunc("/contacts/import", contacts.Handler)
	mux.HandleFunc("/bookmarks", firstparty.Page(bookmarks.Handler, "Bookmarks"))
	mux.HandleFunc("/bookmarks/search", bookmarks.Handler)
	mux.HandleFunc("/client/state", account.ClientStateHandler)
	mux.HandleFunc("/client/services", api.CatalogueHandler)
	mux.HandleFunc("/client/call/", api.AppCallHandler)
	mux.HandleFunc("/client/agents", agent.CatalogueHandler)
	mux.HandleFunc("/services/call/", api.ServiceCallHandler)
	mux.HandleFunc("/services", firstparty.Page(api.ToolsPageHandler, "Services"))
	mux.HandleFunc("/services/", firstparty.Page(api.ServiceRefHandler, "Services"))
	mux.HandleFunc("/service/", firstparty.Page(api.ServiceRefHandler, "Services"))
	mux.HandleFunc("/docs", firstparty.Page(docs.Handler, "Documents"))
	mux.HandleFunc("/notes", firstparty.Page(notes.Handler, "Notes"))
	mux.HandleFunc("/contacts", firstparty.Page(contacts.Handler, "Contacts"))
	mux.HandleFunc("/events", firstparty.Page(events.Handler, "Events"))
	mux.HandleFunc("/files", firstparty.Page(files.Handler, "Files"))
	mux.HandleFunc("/files/", files.Handler)
	mux.HandleFunc("/work", firstparty.Page(work.Handler, "Work"))
	mux.HandleFunc("/apps", firstparty.Page(apps.Handler, "Apps"))
	mux.HandleFunc("/tasks", firstparty.Page(tasks.Handler, "Tasks"))
	mux.HandleFunc("/tasks/", tasks.Handler)
	mux.HandleFunc("/admin", firstparty.Page(admin.Handler, "Admin"))
	mux.HandleFunc("/admin/client", admin.ClientHandler)
	mux.HandleFunc("/admin/config", firstparty.Page(admin.ConfigHandler, "Config"))
	// Populated consumer views catch data-shape and layout regressions.
	views := map[string]any{
		"/markets": map[string]any{"data": []map[string]any{{"symbol": "BTC", "name": "Bitcoin", "price": 62000.25, "change_24h": 2.35, "chart": "https://example.com/chart"}}, "freshness": "Updated just now"},
		"/news":    map[string]any{"feed": []map[string]any{{"id": "sample-news", "title": "A headline with enough detail to wrap cleanly on mobile", "description": "A short news summary.", "category": "Technology", "posted_at": "2026-09-15T10:00:00Z", "url": "https://example.com/story"}}},
		"/video":   map[string]any{"channels": map[string]any{"Technology": map[string]any{"videos": []map[string]any{{"id": "sample-video", "title": "A useful video", "channel": "Sample channel", "published": "2026-09-15T10:00:00Z"}}}}},
		"/blog":    []map[string]any{{"id": "sample-post", "title": "A useful post", "content": strings.Repeat("A short paragraph about a useful idea. ", 100), "author": "App Reader", "tags": "design,apps", "created_at": "2026-09-15T10:00:00Z"}},
	}
	for path, payload := range views {
		mux.HandleFunc(path, firstparty.Page(func(w http.ResponseWriter, r *http.Request) { legacyapp.RespondJSON(w, payload) }, "App"))
	}
	mux.HandleFunc("/mu.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Write([]byte("// fixture"))
	})
	server := httptest.NewServer(webclient.WithData(mux))
	defer server.Close()
	input, _ := json.Marshal(map[string]any{"base": server.URL, "session": session.Token, "older": older.ID})
	cmd := exec.Command("node", "../web/test/apps.cjs")
	cmd.Stdin = strings.NewReader(string(input))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Browser apps: %v\n%s", err, out)
	}
	t.Log(string(out))
}
