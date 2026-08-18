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
	if got := agentTitle("nobody", "news"); got != "News Agent" {
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
