package home

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mu/inbox"
	"mu/internal/auth"
	"mu/internal/thread"
	"mu/service/mail"
)

func TestOverviewKeepsBriefAndConversationOwned(t *testing.T) {
	const owner = "overview_continuity"
	if err := auth.Create(&auth.Account{ID: owner, Name: owner}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(owner)
	if err != nil {
		t.Fatal(err)
	}
	const ref = "<overview-brief@micro.mu>"
	brief := thread.Open(owner, "mail", ref)
	thread.Add(thread.Message{Account: owner, Thread: brief.ID, Ref: ref, Text: "Your appointment is at ten."})
	if err := mail.SendMessageTo(mail.Delivery{From: "Micro", FromID: "agent", ToID: owner, Tag: "brief", MessageID: ref, Body: "Your appointment is at ten.\n\n---\nManage scheduled instructions"}); err != nil {
		t.Fatal(err)
	}
	own := thread.Open(owner, thread.WebClient, "overview-web")
	thread.Add(thread.Message{Account: owner, Thread: own.ID, Text: "Help me plan the journey"})
	foreign := thread.Open("overview-other", thread.WebClient, "overview-private")
	thread.Add(thread.Message{Account: "overview-other", Thread: foreign.ID, Text: "Other account secret"})
	req := httptest.NewRequest("GET", "/home", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	rec := httptest.NewRecorder()
	Handler(rec, req)
	body := rec.Body.String()
	for _, want := range []string{"Your appointment is at ten.", "Continue conversation", "/inbox?id=" + brief.ID, "/inbox?id=" + own.ID, "Continue a conversation"} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q", want)
		}
	}
	for _, unwanted := range []string{"Other account secret", "Manage scheduled instructions", `href="/apps/new"`, `href="/agent/new"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("overview exposes %q", unwanted)
		}
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Errorf("private overview cache policy: %q", got)
	}
	for _, tc := range []struct {
		id     string
		status int
	}{{own.ID, 303}, {foreign.ID, 404}} {
		req := httptest.NewRequest("GET", "/inbox?id="+tc.id, nil)
		req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		rec := httptest.NewRecorder()
		inbox.Handler(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("resume %s: %d, want %d", tc.id, rec.Code, tc.status)
		}
		if tc.status == 303 && rec.Header().Get("Location") != "/agent?session="+own.ID {
			t.Fatal("inbox must resume the exact web conversation")
		}
	}
}

func TestBriefDeliveryDateIsExplicit(t *testing.T) {
	old := time.Date(2026, 9, 12, 9, 30, 0, 0, time.FixedZone("BST", 3600))
	got := briefDeliveredAt(old)
	for _, want := range []string{`datetime="2026-09-12T08:30:00Z"`, "12 September 2026 at 08:30 UTC"} {
		if !strings.Contains(got, want) {
			t.Fatalf("delivery date missing %q: %s", want, got)
		}
	}
	if got := briefDeliveredAt(time.Time{}); !strings.Contains(got, "unavailable") {
		t.Fatal(got)
	}
}
