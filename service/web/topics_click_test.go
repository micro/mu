package web

// Clicking a topic searches for it.
//
// The chips were links to /web?q=<topic> and they did nothing, because this
// handler reads the query from the POST body and not from the URL — on purpose.
// What somebody searches the web for is the example everybody reaches for when
// they say "that is private", so it does not go in a URL, a browser history, or
// the access log of whatever terminates TLS. See Handler and AGENTS.md, "What
// may travel in a URL".
//
// Which makes the obvious fix the wrong one: reading the query string as well
// would have made the chips work by reintroducing exactly the leak the rule
// exists to prevent. They post instead.

import (
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// A topic is a submit button under its own name, in the search form.
func TestATopicPostsRatherThanLinking(t *testing.T) {
	body := renderLanding(t)

	if strings.Contains(body, `href="/web?q=`) {
		t.Error("a topic is still a link with the search term in the URL — it does\n" +
			"not work, and on the way to not working it writes what somebody\n" +
			"searched for into their history and into the TLS terminator's log")
	}
	if !strings.Contains(body, `name="topic"`) {
		t.Error("topics do not post: nothing on the landing page submits a topic")
	}
	// Its own name, because a button called q beside a text input called q
	// sends both and the empty input is the one PostFormValue returns.
	if strings.Contains(body, `<button type="submit" class="recent-search-item" name="q"`) {
		t.Error(`a topic button is named q, so it is submitted alongside the empty ` +
			`text input and the empty one wins`)
	}
	// And inside the form, or the button submits nothing.
	if i, j := strings.Index(body, `name="topic"`), strings.Index(body, `</form>`); i > j {
		t.Error("the topic chips are outside the search form, so clicking one\n" +
			"submits nothing")
	}
}

// And the handler reads it.
func TestAPostedTopicIsTheQuery(t *testing.T) {
	form := url.Values{"topic": {"Kyiv strikes"}}
	r := httptest.NewRequest("POST", "/web", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if got := strings.TrimSpace(r.PostFormValue("q")); got != "" {
		t.Fatalf("q is %q on a topic post, so this proves nothing", got)
	}
	if got := strings.TrimSpace(r.PostFormValue("topic")); got != "Kyiv strikes" {
		t.Fatalf("topic did not survive the round trip: %q", got)
	}

	// The fallback itself, read from the source: exercising the handler needs a
	// search provider, and the branch is one line.
	src := readSearchSource(t)
	if !strings.Contains(src, `query = strings.TrimSpace(r.PostFormValue("topic"))`) {
		t.Error("the handler never reads the posted topic, so a chip posts and\n" +
			"gets the landing page back")
	}
}

func renderLanding(t *testing.T) string {
	t.Helper()
	topicCache.Lock()
	topicCache.topics = []string{"Kyiv strikes", "Nepal floods"}
	topicCache.updated = time.Now()
	topicCache.Unlock()

	r := httptest.NewRequest("POST", "/web", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	Handler(w, r)
	return w.Body.String()
}

func readSearchSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("search.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
