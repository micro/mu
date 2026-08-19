package agent

// The specialists this instance provides, where somebody can find them.

import (
	"strings"
	"testing"
)

// Eleven agents have been in the registry since the router was written, and
// every one was reachable at agent+news@ and nowhere else — not on a page, not
// at a URL, not in the picker. A product that provides services and tools and
// then expects you to build your own agents has the same gap as one that
// expects you to build your own tools.
func TestOurAgentsListsTheOnesWeProvide(t *testing.T) {
	got := defaultRow()

	for _, name := range PlatformNames() {
		a := Platform(name)
		if a == nil {
			t.Fatalf("%s is registered and does not resolve", name)
		}
		if !strings.Contains(got, ">"+a.Name+"<") {
			t.Errorf("%s is not on the page", a.Name)
		}
		if !strings.Contains(got, `href="/agent/`+strings.ToLower(name)+`"`) {
			t.Errorf("%s has no page to open", name)
		}
	}
	// More than the one it listed before.
	if n := strings.Count(got, "agent-row"); n < 5 {
		t.Errorf("only %d agents are listed", n)
	}
}

// A name in a path reaches one of them. /agent/news 404ed while agent+news@
// answered, which is the same agent unreachable by the address people can
// actually see.
func TestAPlatformAgentHasAPage(t *testing.T) {
	id, ok := BySlug("nobody", "news")
	if !ok {
		t.Fatal("/agent/news does not resolve")
	}
	if id != "news" {
		t.Errorf("it resolved to %q", id)
	}
	if got := agentTitle("nobody", "news"); got != "News" {
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
	a := resolveAgent("somebody", "markets", false)
	if a == nil {
		t.Fatal("the markets agent does not resolve for a run")
	}
	if a.ID != "markets" {
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
	if got := toolWords(nil); got != "everything you can reach" {
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

	if got := inboxAddress("nobody", "weather"); got != "agent+weather@example.test" {
		t.Errorf("the weather agent's page says %q", got)
	}
	// The default keeps the bare address, which is what it answers at.
	if got := inboxAddress("nobody", ""); got != "agent@example.test" {
		t.Errorf("the default says %q", got)
	}
}

// And "How to reach it" is about the agent it was clicked from. Every row's
// link fell through to the default panel and said Micro.
func TestConnectKnowsThePlatformAgent(t *testing.T) {
	a := Platform("markets")
	if a == nil {
		t.Fatal("no markets agent")
	}
	got := platformPanel(a, "https://example.test")
	for _, want := range []string{"Markets", "markets_list", "/agent/markets"} {
		if want == "markets_list" {
			// Named as its service rather than its tool.
			want = "markets"
		}
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
