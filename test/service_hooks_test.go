package test

// A function variable is an import the compiler cannot see.
//
// layering_test.go asserts the direction things point, and it does it by
// reading import statements. That is the right rule checked in the wrong place,
// because every one of the edges it forbids has been made anyway — as an
// exported func variable a service declares and internal/server/hooks.go fills
// in at boot. The import is gone, the dependency is not, and the test passes.
//
// Two rules already enforced elsewhere are being walked around exactly this
// way, today:
//
//   - TestNoServiceImportsTheAccount passes, and service/apps depends on
//     account/ through three hooks, one of them wired straight to
//     account.ChargeAppUse.
//   - TestServicesDoNotImportEachOther passes with an allowlist that is empty
//     and, in its own words, "should stay that way" — and service/news depends
//     on service/social through news.FetchSocialContext.
//
// So this counts them. It does not forbid them: some are genuine debt with no
// cheaper answer today, and a test that failed on all nine would be deleted
// within a week. What it does is stop the number going up quietly.
//
// The list below is the ledger AGENTS.md says hooks.go is. It was not: the doc
// named four hooks, there were nine, and one of the four it named
// (stream.AIReplyHook) had already been deleted. A ledger that is wrong in both
// directions tells you the problem is bounded when it is not.
//
// Adding a hook means adding a line here, which is the point. It is a decision
// with a cost, and the cost should be paid where somebody has to type it.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// hookDecl matches an exported package-level function variable.
var hookDecl = regexp.MustCompile(`(?m)^var ([A-Z][A-Za-z0-9]*) func\(`)

// why is what kind of debt each hook is, which is not the same for all of them.
//
// The classification matters more than the count. A service announcing that
// something happened is fine and wants no fixing; a service reaching up into
// the agent is the direction the layering exists to forbid; a service reaching
// sideways is a rule that is enforced elsewhere and is being walked around.
type why int

const (
	// reachesAgent: a service calling up into agent/. The direction has a
	// reason rather than a convention behind it — a service answers a question
	// about state, an agent decides which question to ask, and a service
	// calling an agent is asking the model what its own answer should be.
	//
	// The fix is the one service/mail already made: publish the fact, let the
	// agent subscribe. internal/event exists and agent/mail is the worked
	// example.
	reachesAgent why = iota

	// reachesAccount: a service calling up into account/. Forbidden by
	// TestNoServiceImportsTheAccount, which these route around. What a service
	// is supposed to know about money is internal/quota — prices, not balances
	// — and where quota cannot answer, that is a gap in quota rather than a
	// licence here.
	reachesAccount

	// reachesService: a service calling sideways into another service.
	// Forbidden by TestServicesDoNotImportEachOther, whose allowlist is empty.
	// Whatever two services share belongs in internal/.
	reachesService

	// announces: a service saying something happened, with no opinion about
	// who listens. Not a leak in the same sense — the direction is outward
	// rather than upward — but a hook is still the wrong shape for it, because
	// exactly one listener can ever be attached. internal/event takes any
	// number.
	announces

	// adapter: a provider plugged in underneath, filled only when that provider
	// is configured. This is downward and correct: the service owns the domain,
	// the adapter owns the vendor.
	adapter
)

// hooks is every exported function variable under service/, and what it is.
//
// Sorted by package then name, which is the order the scan produces, so a
// failure reads as a diff rather than as a puzzle.
var hooks = map[string]why{
	// service/apps
	"apps.ChargeQuota": reachesAccount,
	"apps.ChargeUse":   reachesAccount,
	"apps.QuotaCheck":  reachesAccount,

	// service/events
	"events.OnCreate": announces,
	"events.OnFire":   announces,

	// service/news
	"news.FetchSocialContext": reachesService,
}

// Every hook is on the ledger, and everything on the ledger still exists.
//
// Both directions, because the doc failed in both: it under-counted, and it
// named one that had been deleted. A stale entry is not harmless — it is a line
// of debt somebody has already paid being counted again.
func TestEveryServiceHookIsOnTheLedger(t *testing.T) {
	found := scanHooks(t)

	for name := range found {
		if _, known := hooks[name]; !known {
			t.Errorf("%s is a new hook into a service and is not on the ledger.\n"+
				"A function variable is an import the compiler cannot see: adding one\n"+
				"makes a dependency the layering tests will not report. If it is\n"+
				"genuinely the cheapest answer, add it to hooks in\n"+
				"test/service_hooks_test.go with what kind of debt it is — and if it\n"+
				"is not, prefer publishing the fact on internal/event and letting\n"+
				"whoever cares subscribe. See service/mail and agent/mail.", name)
		}
	}

	for name := range hooks {
		if !found[name] {
			t.Errorf("%s is on the ledger and no longer exists — remove the line.\n"+
				"AGENTS.md carried stream.AIReplyHook for exactly this reason, long\n"+
				"after it was deleted.", name)
		}
	}
}

// No service calls an agent.
//
// This was written expecting three — tasks.RunAgent, events.RunAgent and
// events.OnFireEvent, the list AGENTS.md carried as "not yet enforced" — and
// they went in the same change that added this test. Writing the count down was
// what made it obvious they were one thing: four ways to ask an agent for work,
// three of them a service reaching upward through a function variable.
//
// Zero, and it stays zero. A service announces that work was asked for and
// agent/work subscribes; see internal/event.WorkForAgent.
func TestNoNewServiceCallsAnAgent(t *testing.T) {
	const known = 0

	var up []string
	for name, kind := range hooks {
		if kind == reachesAgent {
			up = append(up, name)
		}
	}
	sort.Strings(up)

	if len(up) > known {
		t.Errorf("%d services now call an agent, want %d: %s\n"+
			"A service answers a question about state; an agent decides which\n"+
			"question to ask. A service calling an agent is asking the model what\n"+
			"its own answer should be. Publish the fact instead — see\n"+
			"internal/event, EventMailForAgent, and agent/mail as the worked\n"+
			"example.", len(up), known, strings.Join(up, ", "))
	}
}

// The two rules that are already enforced against imports, checked against
// hooks as well.
//
// These are not a count that may hold steady. TestNoServiceImportsTheAccount
// and TestServicesDoNotImportEachOther both assert zero and both pass, so the
// dependencies below exist in defiance of a rule this repository already
// decided. They are here to be visible until they are gone, and the numbers
// are the only reason the test is not simply red.
func TestTheRulesAlreadyEnforcedAreNotWalkedAround(t *testing.T) {
	counts := map[why]int{}
	names := map[why][]string{}
	for name, kind := range hooks {
		counts[kind]++
		names[kind] = append(names[kind], name)
	}
	for _, list := range names {
		sort.Strings(list)
	}

	// service/apps pays for an app's author out of the payer's balance, which
	// internal/quota cannot express: quota holds prices and deliberately does
	// not know what a balance is. That is a gap in quota, and until it is
	// filled these three stay.
	if n := counts[reachesAccount]; n > 3 {
		t.Errorf("%d service hooks reach the account, up from 3: %s\n"+
			"TestNoServiceImportsTheAccount asserts zero imports and passes. A\n"+
			"hook is how that assertion is avoided.", n, strings.Join(names[reachesAccount], ", "))
	}

	// news asks social what is being said about an article. Whatever two
	// services share goes in internal/, never in a hook and never in a
	// non-service directory under service/.
	if n := counts[reachesService]; n > 1 {
		t.Errorf("%d service hooks reach another service, up from 1: %s\n"+
			"TestServicesDoNotImportEachOther has an empty allowlist and its own\n"+
			"comment says it should stay that way. A sideways hook is a sideways\n"+
			"import the compiler cannot see.", n, strings.Join(names[reachesService], ", "))
	}
}

// scanHooks reads every exported function variable under service/, keyed
// package.Name.
func scanHooks(t *testing.T) map[string]bool {
	t.Helper()

	found := map[string]bool{}
	root := filepath.Join("..", "service")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// The directory is the package — one directory per service, which is
		// the rule the catalogue rests on.
		pkg := filepath.Base(filepath.Dir(path))
		for _, m := range hookDecl.FindAllStringSubmatch(string(b), -1) {
			found[pkg+"."+m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking service/: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no hooks found at all — this scan is broken, not the tree")
	}
	return found
}
