package test

// apps_run does not return a result, so it must not say it does.
//
// It said "Run JavaScript code in a sandboxed environment and return the
// result. Use for calculations, data processing, or any computation the user
// needs." It stores the code and returns a URL. Nothing executes until somebody
// opens that page, in their own browser, later. An agent calling it to work out
// a number got a link, and then had to explain a link to somebody who asked a
// question.
//
// A tool description is the whole interface an agent has. It is not marketing
// copy and it is not aspiration; it is the contract, and a wrong one sends
// every agent that reads it down the wrong path.

import (
	"regexp"
	"strings"
	"testing"
)

func TestAppsRunDoesNotClaimToReturnAResult(t *testing.T) {
	src := registrationSource(t)
	i := strings.Index(src, `Name: "apps_run"`)
	if i < 0 {
		i = strings.Index(src, `Name:        "apps_run"`)
	}
	if i < 0 {
		t.Fatal("apps_run is no longer registered")
	}
	block := src[i:min(i+2600, len(src))]

	// Only the strings a caller sees. The comments above the registration
	// quote the old wording to explain why it changed, and a check that cannot
	// tell an explanation from a claim would forbid explaining anything.
	var visible strings.Builder
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		visible.WriteString(line)
		visible.WriteString("\n")
	}
	block = visible.String()

	// The claim, in the shapes it would take.
	for _, lie := range []string{
		"and return the result",
		"return the result.",
		"any computation the user needs",
	} {
		if strings.Contains(block, lie) {
			t.Errorf("apps_run claims %q — it returns a URL, and the code runs "+
				"later in somebody's browser", lie)
		}
	}

	// And says what it does give back.
	if !regexp.MustCompile(`(?i)returns a link|get back a URL`).MatchString(block) {
		t.Error("apps_run does not say that it returns a link rather than an answer")
	}

	// The parameter text advertised platform helpers that either do not exist
	// in that page or post to a parent with no listener, so they never resolve.
	for _, dead := range []string{"mu.web.fetch()", "mu.db and mu.store", "mu.ai()"} {
		if strings.Contains(block, dead) {
			t.Errorf("apps_run still advertises %s, which never resolves in an "+
				"ad-hoc run — the promise hangs", dead)
		}
	}
}
