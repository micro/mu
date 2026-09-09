package sms

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/userdb"
)

func TestConversationListSeparatesNumberAndChannel(t *testing.T) {
	setup(t)
	history := []Message{
		{ID: "wa-new", Number: "+447700900111", Channel: "whatsapp", Text: "Latest WhatsApp", At: time.Now()},
		{ID: "sms-new", Number: "+447700900111", Text: "Latest SMS", Direction: "out", At: time.Now().Add(-time.Minute)},
		{ID: "other", Number: "+447700900222", Text: "Other person", At: time.Now().Add(-2 * time.Minute)},
		{ID: "wa-old", Number: "+447700900111", Channel: "whatsapp", Text: "Older WhatsApp", At: time.Now().Add(-3 * time.Minute)},
	}
	got := conversationList("conversation-list", history)
	if strings.Count(got, `class="sms-conversation"`) != 3 {
		t.Fatalf("want three conversations: %s", got)
	}
	for _, want := range []string{"Latest WhatsApp", "You: Latest SMS", "Other person", "WhatsApp", "SMS"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(got, "Older WhatsApp") || strings.Contains(got, `name="text"`) {
		t.Error("list rendered full history or reply composers")
	}
	if strings.Index(got, "wa-new") > strings.Index(got, "sms-new") {
		t.Error("latest conversation should come first")
	}
	if strings.Contains(got, `href="/sms?id=+`) {
		t.Error("phone number in conversation URL")
	}
}

func TestConversationPageReadsOnlySelectedOwnedChannel(t *testing.T) {
	setup(t)
	const who = "conversation_owner"
	if err := auth.Create(&auth.Account{ID: who, Approved: true}); err != nil {
		t.Fatal(err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatal(err)
	}
	sms := Record(who, "in", "+447700900111", "Private SMS history", 1)
	wa := RecordOn(ChannelWhatsApp, who, "in", "+447700900111", "Private WhatsApp history", 1)
	foreign := Record("somebody-else", "in", "+447700900111", "Foreign history", 1)
	for _, tc := range []struct {
		id, want, absent, channel string
		status                    int
	}{
		{sms.ID, "Private SMS history", "Private WhatsApp history", "", 200},
		{wa.ID, "Private WhatsApp history", "Private SMS history", "whatsapp", 200},
		{foreign.ID, "", "Foreign history", "", 404},
		{"missing", "", "Private SMS history", "", 404},
	} {
		r := httptest.NewRequest(http.MethodGet, "/sms?id="+url.QueryEscape(tc.id), nil)
		r.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
		w := httptest.NewRecorder()
		Handler(w, r)
		got := w.Body.String()
		if w.Code != tc.status {
			t.Fatalf("id %s status %d: %s", tc.id, w.Code, got)
		}
		if strings.Contains(got, tc.absent) {
			t.Errorf("leaked unrelated history for %s", tc.id)
		}
		if tc.status == 200 {
			if !strings.Contains(got, tc.want) || !strings.Contains(got, `name="channel" value="`+tc.channel+`"`) {
				t.Errorf("wrong conversation or reply channel: %s", got)
			}
			if strings.Count(got, `name="text"`) != 1 {
				t.Error("want one reply composer")
			}
		}
	}
}

func TestBusyConversationDoesNotHideAnotherThreadsHistory(t *testing.T) {
	setup(t)
	const who = "conversation-busy"
	Record(who, "in", "+447700900111", "Older separate conversation", 1)
	for i := 0; i < 205; i++ {
		RecordOn(ChannelWhatsApp, who, "in", "+447700900222", "Busy thread", 1)
	}
	Record("other_owner", "in", "+447700900333", "Not yours", 1)
	if _, err := userdb.Create(ns, who, msgs, map[string]interface{}{"number": "+447700900111", "text": "Legacy SMS", "direction": "in", "at": time.Now().Add(-time.Hour).Format(time.RFC3339)}, false); err != nil {
		t.Fatal(err)
	}
	latest, err := recentConversations(who)
	if err != nil || len(latest) != 2 {
		t.Fatalf("busy thread hid another conversation: %+v, %v", latest, err)
	}
	got := conversationHistory(who, "+447700900111", ChannelSMS)
	if len(got) != 2 || got[0].Text != "Older separate conversation" {
		t.Fatalf("history was limited before selecting the conversation: %+v", got)
	}
}
