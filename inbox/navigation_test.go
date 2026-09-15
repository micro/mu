package inbox

import (
	"fmt"
	"html"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestOlderKeepsInboxListAndPage(t *testing.T) {
	const owner = "inbox_paging_regression"
	for i := 0; i < shown+2; i++ {
		arrived(t, owner, "mail", fmt.Sprint(i), "", "writer@example.com", fmt.Sprintf("Arrival %02d", i))
	}
	for _, path := range []string{"/inbox", "/inbox?view=history"} {
		w := httptest.NewRecorder()
		priority(w, httptest.NewRequest("GET", path, nil), owner)
		match := regexp.MustCompile(`<a href="([^"]+)" class="pager-link">Older`).FindStringSubmatch(w.Body.String())
		if len(match) != 2 {
			t.Fatal("missing older link")
		}
		target := html.UnescapeString(match[1])
		u, err := url.Parse(target)
		if err != nil || u.Query().Get("page") != "2" {
			t.Fatalf("broken older URL: %s", target)
		}
		w = httptest.NewRecorder()
		priority(w, httptest.NewRequest("GET", target, nil), owner)
		if n := strings.Count(w.Body.String(), `class="ib-row`); n != 2 {
			t.Fatalf("older page has %d rows, want 2", n)
		}
		if strings.Contains(w.Body.String(), `<article`) {
			t.Fatal("older opened a single item")
		}
	}
}

func TestReaderHasNeighboursAndInlineReply(t *testing.T) {
	const owner = "reader_navigation_regression"
	for i := 0; i < 3; i++ {
		arrived(t, owner, "mail", fmt.Sprint(i), "", "writer@example.com", fmt.Sprintf("Reader %d", i))
	}
	all := arrivals(owner)
	current := all[1]
	w := httptest.NewRecorder()
	conversation(w, httptest.NewRequest("GET", "/inbox?view=history&page=2&id="+current.ID, nil), owner, current.ID)
	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{all[0].ID, all[2].ID, "← Previous", "Next →", `id="inbox-reply"`, `name="on" value="` + current.ID + `"`, `name="body"`, `action="/inbox/new"`, `/inbox?page=2&view=history`} {
		if !strings.Contains(body, want) {
			t.Errorf("reader missing %q", want)
		}
	}
	if strings.Contains(body, `/inbox/new?`) {
		t.Error("reply still navigates to composer")
	}
}

func TestFailedInlineReplyKeepsDraftInReader(t *testing.T) {
	const owner = "inline_draft_regression"
	th := arrived(t, owner, "mail", "draft", "", "writer@example.com", "Original message")
	values := url.Values{"inline": {"1"}}
	r := httptest.NewRequest("POST", "/inbox/new", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	writeOne(w, r, owner, form{On: th.ID, Body: "Keep my draft", Problem: "Send failed"})
	for _, want := range []string{`id="inbox-reply" open`, "Keep my draft", "Send failed", "Original message"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("failed reply lost %q", want)
		}
	}
}
