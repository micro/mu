package test

// A tool description is the contract, so it has to be true.
//
// This test outlived the endpoint it was written for. apps_run said "Run
// JavaScript code in a sandboxed environment and return the result. Use for
// calculations, data processing, or any computation the user needs." It stored
// the code and returned a URL; nothing executed until somebody opened that page,
// in their own browser, later. An agent calling it to work out a number got a
// link, and then had to explain a link to somebody who had asked a question.
//
// Run is gone — these are static pages, the browser runs them, so the verb was
// wrong about the whole service. apps_embed replaced it and inherits the rule,
// because it has the same two ways to lie: claiming the result comes back, and
// advertising helpers that do not work where the answer puts them.

import (
	"regexp"
	"strings"
	"testing"

	"mu/service/apps"
)

func TestAppsEmbedSaysWhatItGivesBack(t *testing.T) {
	if _, gone := apps.Spec.Endpoints["Run"]; gone {
		t.Fatal("apps still declares Run. It ran nothing: it parked a snippet " +
			"of JavaScript for an hour and returned an id while promising a URL")
	}

	ep, ok := apps.Spec.Endpoints["Embed"]
	if !ok {
		t.Fatal("the apps service does not declare Embed")
	}
	doc := ep.Doc

	// The claim apps_run made, in the shapes it would take.
	for _, lie := range []string{
		"and return the result",
		"return the result.",
		"any computation the user needs",
		"runs it in a browser",
	} {
		if strings.Contains(doc, lie) {
			t.Errorf("apps_embed claims %q — it returns markup, and an app runs "+
				"wherever somebody pastes it", lie)
		}
	}

	// And says what it does give back.
	if !regexp.MustCompile(`(?i)\bHTML\b|iframe`).MatchString(doc) {
		t.Error("apps_embed does not say that it gives back the markup")
	}

	// The other half of the old fault: helpers advertised where nothing answers
	// them. On a third-party page there is no bridge, so every mu.* call waits
	// sixty seconds and then fails. The description has to carry that.
	if !strings.Contains(doc, "mu.") {
		t.Error("apps_embed does not say that mu. only reaches this instance " +
			"from a page on it — off-site those calls hang and then fail")
	}
}
