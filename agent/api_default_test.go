package agent

// /agent is an address, the same way agent@ is.

import (
	"os"
	"strings"
	"testing"
)

// The shortest way for a program to ask this instance a question was POST
// /agent/micro — which names the default agent, and which one that is happens
// to be the single thing a caller should not need to know.
//
// Writing to agent@<domain> has never needed a name: the plain address means
// "whatever answers here" and the tag asks for one in particular. See
// mail.SharedAgentAddressFor, where an empty tag resolves to the default for
// exactly this reason. The two doors are the same door and now read the same
// way — agent@ is /agent, agent+news@ is /agent/news.
func TestThePlainPathIsTheDefaultAgent(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	// The path is trimmed of the bare prefix, not only of the prefix with a
	// slash on it — TrimPrefix(path, "/agent/") leaves "/agent" untouched, so
	// the slug came out as "/agent", contained a slash, and 404ed.
	if !strings.Contains(body, `strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/agent")`) {
		t.Error("the bare path is not trimmed, so POST /agent is read as an agent\n" +
			"named \"/agent\" and answers 404")
	}
	if !strings.Contains(body, "agentID = DefaultPlatformAgent") {
		t.Error("an unnamed path does not resolve to the agent that answers here")
	}
	// And a name still reaches a specialist, through the same lookup the page
	// uses, so /agent/research and POST /agent/research cannot disagree.
	if !strings.Contains(body, "agentSlugTarget(accountID, slug)") {
		t.Error("a named agent is no longer resolved the way the page resolves it")
	}
}

// And the card says the address a caller should use.
func TestTheCardOffersThePlainAddress(t *testing.T) {
	src, err := os.ReadFile("../client/client.go")
	if err != nil {
		t.Fatal(err)
	}
	// The address and the worked example, not the prose around them — the
	// comment there explains why it is not /agent/micro and so contains the
	// string it is arguing against.
	for _, want := range []string{
		`Address: "https://" + host + "/agent"`,
		`"curl -X POST https://" + host + "/agent \\\n"`,
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("the card does not offer the plain address (looked for %s)", want)
		}
	}
}
