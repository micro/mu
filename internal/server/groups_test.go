package server

import (
	"mu/internal/auth"
	"mu/internal/group"
	"mu/service/events"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGroupSpacePagesAndActions(t *testing.T) {
	owner := "workspace-owner"
	member := "workspace-member"
	for _, id := range []string{owner, member} {
		auth.SetAccountForTest(&auth.Account{ID: id, Name: id, EmailVerified: true})
		defer auth.RemoveAccountForTest(id)
	}
	g, err := group.Create(owner, "Studio & friends", false)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(g.ID, owner, "delete", "", "")
	_ = group.Change(g.ID, owner, "invite", member, "")
	_ = group.Change(g.ID, member, "accept", "", "")
	sessions := map[string]string{}
	for _, id := range []string{owner, member} {
		s, _ := auth.CreateSession(id)
		sessions[id] = s.Token
	}
	request := func(who, method, path string, values url.Values) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if who != "" {
			r.AddCookie(&http.Cookie{Name: "session", Value: sessions[who]})
		}
		if method == "POST" {
			r.Header.Set("X-CSRF-Token", auth.CSRFToken(r))
		}
		return r
	}
	base := "/groups?id=" + g.ID
	memberPage := httptest.NewRecorder()
	groupsHandler(memberPage, request(member, "GET", base+"&tab=settings", nil))
	if strings.Contains(memberPage.Body.String(), "Delete group</button>") {
		t.Fatal("member shown owner deletion control")
	}
	for _, tab := range []string{"", "details", "events", "agents", "members", "budget", "settings"} {
		w := httptest.NewRecorder()
		groupsHandler(w, request(owner, "GET", base+"&tab="+tab, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Studio &amp; friends") {
			t.Fatalf("tab %q: %d %s", tab, w.Code, w.Body.String())
		}
		for _, link := range []string{`href="/home"`, `href="/inbox"`, `href="/agents"`, `href="/services"`, `<span>Inbox</span>`, `href="/groups" aria-current="page"`} {
			if !strings.Contains(w.Body.String(), link) {
				t.Fatalf("group page lost global navigation: %s", link)
			}
		}
		if tab == "members" && strings.Contains(w.Body.String(), "Delete group</button>") {
			t.Fatal("deletion belongs in settings, not members")
		}
		if (tab == "settings" || tab == "budget") && !strings.Contains(w.Body.String(), "Delete group</button>") {
			t.Fatal("owner cannot find delete group")
		}
		if strings.Contains(w.Body.String(), ">Budget</a>") {
			t.Fatal("separate budget tab remains")
		}
		if (tab == "settings" || tab == "budget") && !strings.Contains(w.Body.String(), "<h2>Budget</h2>") {
			t.Fatal("settings missing budget")
		}
		if strings.Contains(w.Body.String(), "@groups.") {
			t.Fatal("protocol address exposed as group identity")
		}
		if dir := os.Getenv("GROUP_PREVIEW_DIR"); dir != "" {
			_ = os.MkdirAll(dir, 0700)
			name := tab
			if name == "" {
				name = "conversations"
			}
			_ = os.WriteFile(filepath.Join(dir, name+".html"), w.Body.Bytes(), 0600)
		}
	}
	failedFund := httptest.NewRecorder()
	groupsHandler(failedFund, request(member, "POST", base, url.Values{"id": {g.ID}, "action": {"fund"}, "amount": {"1"}}))
	if !strings.Contains(failedFund.Body.String(), "<h2>Budget</h2>") {
		t.Fatal("failed funding left Settings")
	}
	w := httptest.NewRecorder()
	groupsHandler(w, request("", "GET", base, nil))
	if w.Code == 200 {
		t.Fatal("guest read group")
	}
	w = httptest.NewRecorder()
	groupsHandler(w, request("", "GET", base+"&join=1", nil))
	if w.Code != 200 {
		t.Fatal("invite landing unavailable signed out")
	}
	w = httptest.NewRecorder()
	groupsHandler(w, request(member, "POST", base, url.Values{"id": {g.ID}, "action": {"event"}, "title": {"Private"}, "when": {"2030-10-18T08:00"}, "zone": {"Europe/London"}}))
	if len(events.List(group.Scope(g.ID))) != 0 {
		t.Fatal("member created schedule without admin permission")
	}
	w = httptest.NewRecorder()
	groupsHandler(w, request(owner, "POST", base, url.Values{"id": {g.ID}, "action": {"event"}, "title": {"Team call"}, "when": {"2030-10-18T08:00"}, "zone": {"Europe/London"}}))
	if w.Code != 303 || len(events.List(group.Scope(g.ID))) != 1 || len(events.List(owner)) != 0 {
		t.Fatal("event not isolated to group", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	groupsHandler(w, request(member, "POST", base, url.Values{"id": {g.ID}, "action": {"conversation"}, "name": {"Launch"}, "agent": {"none"}}))
	if w.Code != 303 || !strings.Contains(w.Header().Get("Location"), "/chat?id=group_") {
		t.Fatal("could not create conversation", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	groupsHandler(w, request(member, "POST", base, url.Values{"id": {g.ID}, "action": {"conversation"}, "name": {"Secret"}, "agent": {"foreign-agent"}}))
	cs, _ := group.Conversations(g.ID, member)
	if len(cs) != 2 {
		t.Fatal("foreign agent admitted")
	}
}

func TestCompactGroupInvitationPreview(t *testing.T) {
	g, err := group.Create("compact-preview-owner", "Private team", false)
	if err != nil {
		t.Fatal(err)
	}
	defer group.Change(g.ID, g.Owner, "delete", "", "")
	token, err := group.InvitationLink(g.ID, g.Owner, false)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/join", strings.NewReader(url.Values{"action": {"preview"}, "token": {token}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	groupJoinHandler(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Private team") {
		t.Fatal(w.Code, w.Body.String())
	}
}
