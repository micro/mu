package test

// What a package exports, named so the call site reads.
//
// Services are named for domains and tools are service_method, both written
// down and both enforced. The 1,300 exported functions underneath had no rule
// at all, and it showed: a predicate called ToolDoor, another called
// X402Enabled in package x402, a question called Skip that looked like a
// command.
//
// Only one of the rules in AGENTS.md is mechanically checkable, and it is the
// one that goes wrong most quietly — a name that repeats its own package. The
// call site already says the package, so x402.X402Enabled() says it twice and
// markets.MarketsHTML() reads like a typo. There is no judgement in it, which
// is exactly why it can be a test rather than a habit.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// exported finds a package-level exported name. Methods are excluded: a
// method reads off its receiver, so (*Verified).Settle repeats nothing.
//
// Three shapes, because for a long time it was one.
//
// It matched functions only, and a whole family of constants sat behind that
// gap: internal/event exported EventMailReceived, EventWorkForAgent,
// EventAccountCreated and ten more, every one of them saying its package twice
// at the call site. Nobody noticed for as long as the family was uniform —
// each new one was copied from the last, which is how a convention violation
// becomes a convention.
//
// A const inside a parenthesised block is indented, so the anchor cannot be
// ^; it is a tab or spaces, then the name, then whitespace and '='. That is
// loose enough to match a package-level `var x = ...` too, which is fine —
// the rule applies to those as well.
var exported = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^func ([A-Z]\w*)\s*\(`),
	regexp.MustCompile(`(?m)^[ \t]+([A-Z]\w*)\s+=\s`),
}

// Types are not checked yet, and that is a decision rather than an oversight.
//
// Adding `^type ([A-Z]\w*)` finds twelve more, and they do not divide cleanly:
//
//	service/apps.AppSearchRequest, .AppSearchResponse
//	service/places.PlacesResponse
//	service/prayer.PrayerTimes
//	service/social.SocialContext
//	service/wallet.WalletAuth
//	service/weather.WeatherForecast
//	admin.AdminFlag
//	internal/user.UserPost, .UserApp
//	internal/flag.FlaggedItem
//	internal/cli.Client
//
// The first nine are real and the fix is mechanical. The last two are not:
// "Flagged" is a word rather than "flag" plus a suffix, and cli.Client only
// matches because "Cli" happens to be the first three letters of "Client" —
// the check is a prefix test and has no idea where a word ends.
//
// AGENTS.md already says how this repository handles that case, about the
// nineteen names that stutter at the end: "The rule is the same rule and the
// fix is the same fix; the reason it is written down rather than done is that
// some of them are not stutters at all and each needs looking at." Written
// down here for the same reason, rather than parked in stutterAllowed — that
// map is for names where the repeat is a word, and burying nine real stutters
// in it would make the rule mean nothing.

// stutterAllowed are the names where the repeat is a word rather than the
// package saying itself twice.
//
// Services() is the plural of the package's subject, not "service" plus
// "s" — same for Agents() and Flags(). Geocode is one word. Each of these has to be
// looked at rather than pattern-matched, which is why the list is short and
// should stay short: if it grows, the rule is wrong.
var stutterAllowed = map[string]bool{
	"internal/service.Services": true,
	"agent.Agents":              true,
	"agent.AgentsHandler":       true,
	"internal/geo.Geocode":      true,
	"internal/flag.Flags":       true,
}

func TestExportedNamesDoNotStutter(t *testing.T) {
	var checked, stutters int

	for _, root := range []string{"home", "agent", "service", "client", "admin", "account", "tool", "internal"} {
		err := filepath.Walk(at(root), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			dir, _ := filepath.Rel(at(""), filepath.Dir(path))
			pkg := filepath.Base(dir)

			var names []string
			for _, re := range exported {
				for _, m := range re.FindAllStringSubmatch(string(b), -1) {
					names = append(names, m[1])
				}
			}
			for _, name := range names {
				checked++

				if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(pkg)) {
					continue
				}
				if len(name) == len(pkg) {
					continue // the name is the package's own word, e.g. Account()
				}
				if stutterAllowed[dir+"."+name] {
					continue
				}
				stutters++
				// Case-insensitively, because the prefix matched that way: pkg is
				// lower case and the name is exported, so TrimPrefix strips
				// nothing and the suggestion repeats the mistake back.
				t.Errorf("%s.%s repeats its package — the call site already says %q, "+
					"so this reads it twice. %s.%s says the same thing",
					dir, name, pkg, pkg, name[len(pkg):])
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if checked < 800 {
		t.Fatalf("only %d exported functions scanned — this is broken, not the code", checked)
	}
	_ = stutters
}

// actionReportsOutcome are the bool-returning verbs that are not predicates.
//
// This is the distinction the rule turns on and it cannot be seen from the
// name alone: an action that reports whether it happened is a perfectly good
// verb returning a bool — ok := doThing() — and only a *question* shaped like
// an instruction is the mistake. Generate starts a run and says whether it
// started; WritePaymentRequired writes the 402 and says whether it wrote one.
//
// Each entry is a claim that the function does something, so a new one has to
// be argued for rather than added to make the build pass.
var actionReportsOutcome = map[string]bool{
	"agent/digest.Generate":                 true,
	"service/mail.StartSMTPServerIfEnabled": true,
	"internal/x402.WritePaymentRequired":    true,
}

// And a question is not a bare verb.
//
// A verb tells; a predicate asks. usage.Skip(path) looked like an instruction
// to skip something and was a question about whether a path is noise, and
// twilio.VerifyInbound() looked like it verified something and reported a
// setting. Both are read at a call site inside an if, where an instruction is
// exactly the wrong thing to see. They are usage.Skipped and
// twilio.VerifiesInbound now.
//
// Only the clear-cut verbs are listed. Deciding in general whether a word is a
// verb needs a dictionary, and a test that guesses would be argued with rather
// than fixed.
func TestAPredicateIsNotAnInstruction(t *testing.T) {
	verbs := []string{
		"Skip", "Verify", "Detect", "Moderate", "Generate", "Fetch", "Delete",
		"Create", "Send", "Write", "Start", "Stop", "Parse", "Render",
		"Subscribe", "Publish", "Register", "Open", "Close", "Mark", "Join",
	}

	boolFunc := regexp.MustCompile(`(?m)^func ([A-Z]\w*)\s*\([^)]*\)\s+bool\s*\{`)
	var checked int

	for _, root := range []string{"home", "agent", "service", "client", "admin", "account", "tool", "internal"} {
		err := filepath.Walk(at(root), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			dir, _ := filepath.Rel(at(""), filepath.Dir(path))

			for _, m := range boolFunc.FindAllStringSubmatch(string(b), -1) {
				name := m[1]
				checked++
				if actionReportsOutcome[dir+"."+name] {
					continue
				}
				for _, v := range verbs {
					// The verb has to be the whole first word, so Settled and
					// Verified — participles, which are what this wants — are
					// not mistaken for Set and Verify.
					if name == v || (strings.HasPrefix(name, v) && len(name) > len(v) &&
						name[len(v)] >= 'A' && name[len(v)] <= 'Z') {
						t.Errorf("%s.%s returns bool and starts with the verb %q — it is "+
							"read inside an if, where an instruction is the wrong thing "+
							"to see. Say it as an adjective, a participle, or a noun "+
							"phrase describing the argument", dir, name, v)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if checked < 50 {
		t.Fatalf("only %d bool-returning functions scanned — this is broken", checked)
	}
}
