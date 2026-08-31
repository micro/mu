package test

// A URL may carry what a thing is called. Never what a person said.
//
// The query string is inside TLS, so this is not about the wire — a middlebox
// sees the SNI hostname and nothing else. It is about the four places a URL
// comes to rest. Two of them we have closed: our own request log records
// r.URL.Path rather than RequestURI (internal/server/serve.go), and every page
// carries <meta name="referrer" content="no-referrer">. Two we cannot. Browser
// history syncs to an account and pasted links go wherever links go; and
// whatever terminates TLS in front of us logs the full URI, which for a
// self-hosted product means the nginx or Caddy the owner put there, both of
// which log the query by default. So an inbox search in a GET is a person's mail
// search written in plaintext to /var/log on their own box, and there is nothing
// we can do about that from in here except not put the words there.
//
// The rule is in AGENTS.md under "What may travel in a URL". This holds the
// line, and its allowlist below is the record of every surface we decided is
// public enough to keep a GET.
//
// # What it looks for
//
// Reads of named parameters out of the URL query: r.URL.Query().Get("q") and
// the handful of others that carry either a person's words or a credential.
// Deliberately not a general "is this handler private" analysis, which would
// need to know what every route means. A parameter name is a decision somebody
// made, in one place, and it is checkable.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The parameters that carry what somebody typed, or a credential.
//
// "id" and "tab" and "page" are not here, and that is the point of the rule
// rather than a gap in it: an id names a thing and a tab names a view, and both
// of those are what a URL is for. What must not travel is the sentence somebody
// wrote into a box.
//
// "token" is absent for one reason, stated so nobody adds it back without
// reading this: /verify?token= is the emailed-link pattern, the token is
// single-use and consumed on arrival, and there is no way to deliver a link by
// mail with the token anywhere but in it. Every other credential is banned.
// r.FormValue is here beside URL.Query().Get because it reads *both* — the body
// and the query — so a form switched to POST while the handler still called
// FormValue would stop putting the words in the URL and go on accepting them
// there, which is the bug still reachable and the test no longer looking. The
// sanctioned read is r.PostFormValue, which is the body only. The leading `\.`
// is what distinguishes them: `.FormValue(` matches and `.PostFormValue(` does
// not, because the character before FormValue there is a `t`.
var privateParams = regexp.MustCompile(
	`(URL\.Query\(\)\.Get|\.FormValue)\("(q|query|prompt|secret|password|passphrase|apikey|api_key)"\)`)

// Where a query in the URL is fine, and why.
//
// Public content, searched by anybody, where the query discloses nothing the
// reader did not already bring — and where a search that cannot be linked to is
// worse than one that can. Every entry here is a decision; adding one means
// deciding the same thing again, in writing.
var publicSearch = map[string]string{
	"service/news/news.go":       "the news index is public and a headline search is not about the reader",
	"service/images/images.go":   "public image search, the same search anybody would run",
	"service/video/video.go":     "public video search",
	"service/docs/handler.go":    "the documentation, which is on the landing page",
	"service/social/social.go":   "the public timeline",
	"service/users/page.go":      "the directory of accounts on this instance, which the strip on Home already names",
	"service/archive/page.go":    "what this instance has collected, which is published",
	"internal/app/chat.go":       "the box on the front page, which searches the public archive",
	"internal/cli/cli.go":        "a CLI building a request, not a server reading one",
	"internal/api/api.go":        "the REST door, where the caller chose the carrier",
	"internal/server/routes.go":  "routing, not a page",
	"tool/tool.go":               "the tool catalogue, which is published at /api",
	"service/apps/apps.go":       "the apps directory, which is public",
	"service/blog/blog.go":       "published posts",
	"service/stream/stream.go":   "the public stream",
	"service/reader/reader.go":   "public feeds",
	"service/wiki/wiki.go":       "published pages",
	"service/prayer/prayer.go":   "public reference text",
	"service/quran/quran.go":     "public reference text",
	"service/bible/bible.go":     "public reference text",
	"service/dictionary/dict.go": "public reference text",

	// Not a search box: a link that asks a question, where the URL *is* the
	// message and there is nowhere else to carry it. Both pages take it out of
	// the address bar with replaceState once it has been used, so it does not
	// sit in the reader's history. The box on each page posts.
	"agent/agent.go": "the ?q= deep link that seeds a question, stripped from history on arrival",
	"home/home.go":   "the same deep link, seeding the box on Home",

	// The JSON branch of /places is a geocoding lookup over public OpenStreetMap
	// data — the same query anybody would make, and the part that is about the
	// person asking is the session, not the term. The page's own form posts to
	// /places/search.
	"service/places/places.go": "geocoding over public data, from the JSON door; the page form posts",

	// A POST handler reading its own body. FormValue would read the query too,
	// which is why these were flagged; both are PostFormValue now and the match
	// here is the word inside the comment explaining that.
	"internal/auth/oauth.go":  "reads PostFormValue; the match is the comment saying why",
	"internal/setup/setup.go": "reads PostFormValue; the match is the comment saying why",
}

func TestNothingPrivateTravelsInAURL(t *testing.T) {
	var offenders []string

	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if n := info.Name(); n == ".git" || n == "node_modules" || n == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hits := privateParams.FindAllStringSubmatch(string(b), -1)
		if len(hits) == 0 {
			return nil
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), "../")
		if _, ok := publicSearch[rel]; ok {
			return nil
		}
		for _, h := range hits {
			offenders = append(offenders, rel+` reads ?`+h[2]+`= out of the URL`)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, o := range offenders {
		t.Errorf("%s.\n"+
			"    A URL carries what a thing is called, never what a person said — "+
			"see AGENTS.md, \"What may travel in a URL\".\n"+
			"    Take it in the body of a POST, or add the file to publicSearch "+
			"in this test with the reason it is public.", o)
	}
}

// And the rule is written down where somebody would look for it.
//
// The test alone is a rule with no explanation: it says a line was crossed and
// not why the line is there, and the next person to hit it needs the reasoning
// more than the verdict.
func TestTheURLRuleIsWrittenDown(t *testing.T) {
	b, err := os.ReadFile("../AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	for _, want := range []string{
		"What may travel in a URL",
		"TestNothingPrivateTravelsInAURL",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("AGENTS.md does not mention %q, so the rule this test enforces "+
				"is only in the test", want)
		}
	}
}
