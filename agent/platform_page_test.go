package agent

// The specialists this instance provides, where somebody can find them.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mu/internal/auth"
)

// A name in a path reaches one of them. /agent/news 404ed while agent+news@
// answered, which is the same agent unreachable by the address people can
// actually see.
func TestAPlatformAgentHasAPage(t *testing.T) {
	withProbe(t)
	id, ok := BySlug("nobody", probeID)
	if !ok {
		t.Fatal("/agent/" + probeID + " does not resolve")
	}
	if id != probeID {
		t.Errorf("it resolved to %q", id)
	}
	if got := agentTitle("nobody", probeID); got != "Probe" {
		t.Errorf("the page would be titled %q", got)
	}
	// And a name that is nothing is still a 404 rather than the default, or a
	// typo quietly serves a different agent.
	if _, ok := BySlug("nobody", "zarquon"); ok {
		t.Error("a name that names nothing resolved")
	}
}

// The chat runs the agent the page names. resolveAgent knew the roster, the
// published ones and the legacy store — not this instance's own — so the page
// said News and the default answered with every tool.
func TestTheChatRunsThePlatformAgent(t *testing.T) {
	withProbe(t)
	a := resolveAgent("somebody", probeID)
	if a == nil {
		t.Fatal("the agent the page names does not resolve for a run")
	}
	if a.ID != probeID {
		t.Errorf("resolved to %q", a.ID)
	}
	if len(a.Tools) == 0 {
		t.Error("it came back with every tool, so its scope was lost")
	}
}

// What an agent reaches, in words. A row of tool names is a list somebody has
// to parse to learn what "news, web" says outright.
func TestToolWordsNamesServicesNotTools(t *testing.T) {
	got := toolWords([]string{"news_list", "news_search", "web_search", "web_fetch"})
	if got != "news, web" {
		t.Errorf("toolWords = %q", got)
	}
	// One word for the unconfined case. It read "everything you can reach",
	// which is a sentence answering a question nobody asked in a cell beside
	// "news, web".
	if got := toolWords(nil); got != "Everything" {
		t.Errorf("an agent with every tool reads %q", got)
	}
}

// An agent's own page says its own address.
//
// The list published agent+weather@ and the weather agent's page said to write
// to the bare agent@ — two addresses for one agent, and the one on its page
// reaches a different one.
func TestAPlatformAgentPageSaysItsOwnAddress(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "example.test")

	withProbe(t)
	if got := inboxAddress("nobody", probeID); got != "agent+"+probeID+"@example.test" {
		t.Errorf("the agent's own page says %q", got)
	}
	// The default keeps the bare address, which is what it answers at.
	if got := inboxAddress("nobody", ""); got != "agent@example.test" {
		t.Errorf("the default says %q", got)
	}
}

// And "How to reach it" is about the agent it was clicked from. Every row's
// link fell through to the default panel and said Micro.
func TestConnectKnowsThePlatformAgent(t *testing.T) {
	a := withProbe(t)
	got := platformPanel(Platform(probeID), "https://example.test")
	// Named as its service rather than its tool: "news", not "news_list".
	for _, want := range []string{a.Name, "news", "/agent/" + probeID} {
		if !strings.Contains(got, want) {
			t.Errorf("the panel is missing %q", want)
		}
	}
	if strings.Contains(got, "the default agent") {
		t.Error("the panel is still describing Micro")
	}
}

// Nothing is called "X Agent" any more — the noun repeated in its own category,
// on a page already headed "Our agents".
func TestAgentNamesDoNotEndInAgent(t *testing.T) {
	for _, name := range PlatformNames() {
		a := Platform(name)
		if a == nil {
			continue
		}
		if strings.HasSuffix(a.Name, " Agent") {
			t.Errorf("%q still ends in Agent", a.Name)
		}
	}
}

// The page is about the agents you made.
//
// It listed this instance's eleven as well, under "Our agents", on the argument
// that they had been reachable at agent+news@ and nowhere else. True, and the
// cure was worse: /agents is where somebody works out what an agent *is*, and
// six rows of things nobody made — with no stated principle behind why news but
// not sport — taught that an agent is something the product hands you rather
// than something you make.
//
// This test used to hold their order relative to yours. What it holds now is
// that they are not there, and that the page leads with making one.
//
// The registry is untouched: platformRow still exists and agent+news@ still
// routes. It is the page that changed, not the roster.
func TestTheRosterIsYourOwnAgents(t *testing.T) {
	const who = "roster_order"
	if err := auth.Create(&auth.Account{ID: who}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}
	sess, err := auth.CreateSession(who)
	if err != nil {
		t.Fatalf("creating the session: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	RosterHandler(rec, req)
	page := rec.Body.String()

	if strings.Contains(page, "Our agents") {
		t.Error("the instance's specialists are listed again — the page is for the " +
			"agent you have and the ones you made")
	}
	// The default is on it, and it is the only one of the instance's that is.
	// Micro is not one of the eleven in the sense the specialists were: it is the
	// agent this account already has, it answers agent@, and it is who the chat
	// talks to. Leaving it off told a new account it had no agents, which is
	// false.
	if !strings.Contains(page, ">Micro</a>") {
		t.Error("the default agent is missing, so a new account is told it has none")
	}
	for _, specialist := range []string{"agent+news@", "agent+markets@", ">Weather</a>"} {
		if strings.Contains(page, specialist) {
			t.Errorf("%s is back on the roster", specialist)
		}
	}
	// And the way to make one is on it.
	if !strings.Contains(page, "New agent") {
		t.Error("there is no way to make an agent on the page about agents")
	}
	// Fork is gone: it offered to copy an agent as a first-class action, on a
	// page where most people have never made one.
	if strings.Contains(page, "?fork=") {
		t.Error("Fork is back on the roster")
	}
}
