package test

// A service reaching a model is an edge the layering test cannot see.
//
// AGENTS.md: "An agent may import a service; a service may never import an
// agent." TestNoNewServiceCallsAnAgent asserts zero and passes. Meanwhile
// services call a model directly through internal/ai, which is substrate a
// service is entitled to know about — so the forbidden act is performed
// through a permitted import and no layering test reports it.
//
// This is the shape agent/moderate was extracted to fix: an analyzer function
// variable inside internal/flag, filled at boot by service/chat, three
// services asking a model what their own answer should be through an edge no
// test could see. The extraction fixed those three. The pattern stayed.
//
// # The line
//
// Not every call is the same act, and a sweep that treated them alike would
// make things worse — it would push image generation behind an agent, which is
// nonsense, and leave the room replies where they are because moving those is
// hard.
//
// The question is whether the model is producing the answer the caller asked
// for, or deciding what the answer should be.
//
//   - "Generate an image of a harbour at dusk" — the caller asked for an image
//     and gets one. The model is the implementation, exactly as an HTTP call to
//     a provider would be. A service using a tool.
//   - "Say something interesting in the Tech room" — nobody asked for those
//     words. The service decided there should be a message and what it says.
//     A service being an agent.
//
// # Why this counts rather than forbids
//
// The same reason service_hooks_test.go counts: a test that failed on all of
// them would be deleted within a week, and the ones that are genuinely fine
// would be "fixed" into something worse. What it stops is the number going up
// quietly, and it records which of them are debt so nobody has to re-derive
// the judgement from the call site.
//
// Moving the judgement cases out is #1469 and, for chat, #89 — where it also
// unblocks XMPP.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// modelCall matches a call into internal/ai that reaches a model.
//
// The three entry points, not the Prompt literal: a prompt built and passed to
// a helper is one call, and counting the literals would report a package that
// shares one path between four methods as four reaches.
var modelCall = regexp.MustCompile(`\bai\.(Ask|AskStream|GenerateImage)\(`)

// kind is what a service is doing when it reaches a model.
type kind int

const (
	// producesAnswer: the model is the answer the caller asked for. A service
	// using a tool, and downward — the service owns the domain, the model is
	// the implementation of one operation in it. Nothing to fix, and this is
	// recorded so it is not "fixed" later.
	producesAnswer kind = iota

	// decidesAnswer: the model is deciding what the service's answer should
	// be, on nobody's request. This is the direction the layering exists to
	// forbid, arriving through internal/ai instead of through agent/.
	//
	// The fix is the one agent/mail is the worked example of: the service
	// publishes the fact, an agent subscribes and decides. internal/event
	// exists for exactly this.
	decidesAnswer
)

// models is every package under service/ that reaches a model, what it is
// doing, and how many call sites it has.
//
// The count is on the ledger so that a second reach inside a package already
// listed fails too. A package appearing here once is not a licence for the
// package.
var models = map[string]struct {
	what  kind
	sites int
}{
	// Builds an app from a description and edits one from an instruction. The
	// caller asked for a program and gets a program; the model is how it is
	// written, the way a compiler is how it is compiled.
	"service/apps":       {producesAnswer, 2},
	"service/apps/micro": {producesAnswer, 2},

	// Generates an image. The clearest case on the list: an image service that
	// cannot make an image is not a service.
	"service/images": {producesAnswer, 2},

	// Summarise, translate, rewrite. Every method is "here is some text, do
	// this to it" — the caller supplies the input and names the operation.
	"service/text": {producesAnswer, 1},

	// Sentiment and topics over articles the caller supplied. A transformation
	// of given input into the thing that was asked for.
	"service/news": {producesAnswer, 1},

	// Extracts trending topics from headlines it is handed. Same shape as
	// news: the input is given and the operation is named.
	"service/web": {producesAnswer, 1},

	// DEBT. Writes a note nobody asked for, on a schedule, in a voice the
	// service chose. That is an editorial decision, and an editorial decision
	// is an agent's. agent/blog already exists and already writes opinion
	// pieces this way.
	"service/blog": {decidesAnswer, 1},

	// DEBT, and the one that matters most. Composes replies in a room and
	// opens conversations — "ask a thought-provoking question" is the service
	// deciding there should be a message and what it says. Named in #1469 and
	// tracked in #89, where moving it out also unblocks serving XMPP: an agent
	// that joins a room as a participant is the same change.
	"service/chat": {decidesAnswer, 1},
}

// Every reach is on the ledger, at the count it actually has.
func TestEveryServiceModelCallIsOnTheLedger(t *testing.T) {
	found := scanModelCalls(t)

	for pkg, n := range found {
		entry, known := models[pkg]
		if !known {
			t.Errorf("%s calls a model and is not on the ledger.\n"+
				"internal/ai is substrate, so the layering tests will not report this:\n"+
				"a service reaching a model through it does the thing\n"+
				"TestNoNewServiceCallsAnAgent forbids, through a permitted import.\n"+
				"If the model is producing the answer the caller asked for, add it to\n"+
				"models in test/service_models_test.go as producesAnswer. If it is\n"+
				"deciding what the answer should be, that belongs in an agent — see\n"+
				"agent/mail for the worked example.", pkg)
			continue
		}
		if entry.sites != n {
			t.Errorf("%s has %d model call(s), the ledger says %d.\n"+
				"A package already on the ledger is not a licence for the package:\n"+
				"each new reach is its own decision and is recorded where somebody\n"+
				"has to type it.", pkg, n, entry.sites)
		}
	}

	// And nothing stale. A ledger entry for debt already paid counts it twice,
	// which is how the hooks doc came to be wrong in both directions.
	for pkg := range models {
		if _, still := found[pkg]; !still {
			t.Errorf("%s is on the ledger and no longer calls a model — "+
				"remove the entry rather than leaving the debt counted", pkg)
		}
	}
}

// The debt does not grow.
//
// Pinned as a number as well as a list, because the list is the thing somebody
// edits and a number is the thing somebody notices. Both go down together or
// neither does.
func TestNoNewServiceDecidesItsOwnAnswerWithAModel(t *testing.T) {
	const known = 2 // service/blog, service/chat — see #1469 and #89

	var debt []string
	for pkg, e := range models {
		if e.what == decidesAnswer {
			debt = append(debt, pkg)
		}
	}
	sort.Strings(debt)

	if len(debt) > known {
		t.Errorf("%d services now decide their own answer with a model, was %d:\n  %s\n"+
			"A service answers a question about state; deciding which question to ask\n"+
			"is an agent's job. Publish the fact on internal/event and let an agent\n"+
			"subscribe — see service/mail and agent/mail.",
			len(debt), known, strings.Join(debt, "\n  "))
	}
	if len(debt) < known {
		t.Errorf("only %d services still decide their own answer with a model, and this\n"+
			"test still says %d. Lower it — a number that is too high stops being a\n"+
			"limit: %s", len(debt), known, strings.Join(debt, ", "))
	}
}

// scanModelCalls counts calls into a model, per package under service/.
func scanModelCalls(t *testing.T) map[string]int {
	t.Helper()

	found := map[string]int{}
	err := filepath.Walk(at("service"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := len(modelCall.FindAllIndex(b, -1)); n > 0 {
			// Package-relative, so the ledger reads as import paths do.
			dir := filepath.ToSlash(filepath.Dir(path))
			if i := strings.Index(dir, "service/"); i >= 0 {
				dir = dir[i:]
			}
			found[dir] += n
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) == 0 {
		t.Fatal("no model calls found anywhere under service/ — the call shape has " +
			"changed and this test is reading for something that no longer exists")
	}
	return found
}
