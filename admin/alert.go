package admin

// Telling the operator when something is worth knowing.
//
// An instance that is running is an instance nobody is looking at. The
// admin pages answer every question here — usage, spend, the log, the
// diagnostics — and answering a question is not the same as raising one: you
// have to already suspect something to go and look, and the things worth
// knowing are exactly the ones you did not suspect.
//
// # Facts below, judgement here
//
// Nothing under service/ or internal/ decides that something is notable.
// internal/auth publishes that an account was created and knows nothing about
// who cares — see event.AccountCreated, and the note there about why a
// building block must not encode a product requirement. This package is where
// "an account was created" becomes "wake somebody", because the operator is a
// product decision and admin is where the operator lives.
//
// # Two shapes of thing, two mechanisms
//
// Discrete facts arrive on the bus: an account was created. There is nothing to
// poll and no threshold to pick, and the alert is the fact.
//
// Levels and rates do not arrive at all — nothing publishes "the disk is 91%
// full", because being 91% full is not an event, it is a state somebody has to
// go and look at. Those are read on a timer and compared against a threshold an
// operator set. Trying to make them events would mean a publisher deciding what
// counts, which is the judgement this package exists to hold.
//
// # Why the inbox
//
// Delivery is mail to the admin's own mailbox here, which lands in
// internal/thread and shows up at /inbox like anything else. Not a new channel:
// the product's claim is that an address is the smallest interface there is, and
// an instance that needed its own private notification pipe to tell its operator
// something would be arguing against itself. It also means the alert threads,
// is searchable through recall, and can be replied to — and a reply to a
// notice about a spike goes to the agent, which has the tools to go and look.
//
// # The cooldown is the feature
//
// An alert that repeats every tick is noise, and noise is indistinguishable
// from nothing within about two days. Every alert has a key, and a key that
// fired recently does not fire again — see fired. That is why thresholds are
// safe to set aggressively: crossing one costs one message, not one a minute
// until you fix it.

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mu/account"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/origin"
	"mu/internal/push"
	"mu/internal/settings"
	"mu/internal/usage"
	"mu/service/mail"
)

// Watch starts the watcher. Called at boot.
func Watch() {
	// No signup alert.
	//
	// This raised one on every AccountCreated, which is a growth metric: it is
	// interesting to somebody running a service other people are joining and to
	// nobody else. On the instance almost everybody actually has — their own,
	// with one account on it — the first thing that ever happened was being
	// told that they had signed up, about themselves, in the voice of an
	// operator watching a funnel.
	//
	// Which is the general rule this is the first instance of: micro.mu is an
	// instance, not a special build, so anything only true when we are the host
	// does not belong compiled into everybody's binary. What is left below is
	// the operational half — rates and a disk filling — which is true for
	// whoever is running it.

	go func() {
		// Once shortly after boot, so an instance that has been down through
		// something interesting says so rather than waiting out a full period.
		// Not immediately: usage.Load and the services are still settling.
		time.Sleep(settleFor)
		for {
			sweep()
			time.Sleep(checkEvery)
		}
	}()
}

const (
	// settleFor is how long to leave the instance alone after boot.
	settleFor = 2 * time.Minute
	// checkEvery is how often the levels are read. Coarse on purpose: these are
	// hourly rates and a disk that is filling, neither of which changes in a
	// minute, and every sweep walks the accounts.
	checkEvery = 5 * time.Minute
)

// alert is one thing worth telling somebody.
//
// Why is not decoration. An operator reading "usage is high" at 3am has to
// reconstruct what the threshold was, what it is now and what they might do —
// and the moment that reconstruction is work, the next alert gets ignored. The
// message carries it.
type alert struct {
	Key   string // what must not repeat while it is still true
	What  string // the subject line
	Why   string // the number, the threshold, and what it means
	Where string // a page that shows more
}

// sweep reads the levels and raises what has crossed.
func sweep() {
	instanceRate()
	accountRates()
	newCallers()
	diskFilling()
	somethingWrong()
}

// somethingWrong is the instance telling on itself.
//
// app.Alert has been recording "things this instance did that it should not
// have had to, or refused in order to protect itself" for a long time, and the
// only way to find out was to open /admin/log and look. That is the exact shape
// of the problem this package exists for: the entries were being written all
// along and nobody was reading them, so a key store refusing a write was
// indistinguishable from a quiet afternoon.
//
// So they are a source here rather than a second, parallel idea of what an
// alert is. Nothing new is recorded — this reads the count and notices it
// moving. The message names how many and sends you to the list; enumerating
// them in a mail would be a log with worse formatting.
func somethingWrong() {
	n := app.AlertCount()
	wrongMu.Lock()
	was := lastWrong
	lastWrong = n
	first := !wrongSeen
	wrongSeen = true
	wrongMu.Unlock()

	// The first sweep after a restart establishes the baseline rather than
	// announcing whatever was already there. Same argument as seeding the
	// callers file: an instance that comes back up with a full list should not
	// treat all of it as new.
	if first || n <= was {
		return
	}
	raise(alert{
		Key:  "wrong",
		What: fmt.Sprintf("%d new thing(s) this instance had to refuse or work around", n-was),
		Why: fmt.Sprintf("%d in the held list now. These are the things it logs when it "+
			"protects itself — a store refusing a write, a limit hit, something arriving "+
			"malformed. They are kept apart from the rolling log so they do not scroll away.", n),
		Where: "/admin/log",
	})
}

var (
	wrongMu   sync.Mutex
	lastWrong int
	wrongSeen bool
)

// instanceRate is how busy the whole instance is.
func instanceRate() {
	limit := number(settings.Get("ALERT_CALLS_PER_HOUR"), 5000)
	if limit <= 0 {
		return
	}
	n := usage.TotalOver(usage.Hour, 1)
	if n < limit {
		return
	}
	raise(alert{
		Key:  "rate:instance",
		What: fmt.Sprintf("Busy: %s calls in the last hour", usage.HumanCount(n)),
		Why: fmt.Sprintf("The threshold is %s an hour (ALERT_CALLS_PER_HOUR). This is "+
			"the whole instance, so it is either real traffic or one caller in a loop — "+
			"the per-account list says which.", usage.HumanCount(limit)),
		Where: "/admin/usage",
	})
}

// accountRates is one caller doing much more than the rest.
//
// The thing this catches that the instance rate does not: a single agent in a
// retry loop is expensive long before it is a fifth of the instance's traffic,
// and on a small instance it will never be a fifth of anything.
func accountRates() {
	limit := number(settings.Get("ALERT_ACCOUNT_CALLS_PER_HOUR"), 1000)
	if limit <= 0 {
		return
	}
	for _, acc := range auth.AllAccounts() {
		n := usage.TotalForOver(acc.ID, usage.Hour, 1)
		if n < limit {
			continue
		}
		raise(alert{
			Key:  "rate:" + acc.ID,
			What: fmt.Sprintf("%s made %s calls in the last hour", acc.ID, usage.HumanCount(n)),
			Why: fmt.Sprintf("The threshold is %s an hour per account "+
				"(ALERT_ACCOUNT_CALLS_PER_HOUR). One account at this rate is usually a "+
				"loop rather than a person.", usage.HumanCount(limit)),
			Where: "/admin/usage",
		})
	}
}

// newCallers is an account calling tools for the first time.
//
// Signing up and using it are different events and the second is the one that
// means something. Somebody who signed up three weeks ago and has just made
// their first call is the most interesting line an operator can read, and
// nothing on any page would ever have shown it.
//
// The set is on disk rather than in memory, or a restart re-announces everybody
// who has ever called anything.
//
// And it is seeded silently the first time. Without that, switching this on for
// an instance that already has twenty active accounts announces all twenty at
// once — twenty messages saying "started using the API" about people who
// started months ago, which is the exact noise that teaches somebody to ignore
// the channel. The first sweep records who has called and says nothing; every
// caller after that is genuinely new to us.
func newCallers() {
	known, seeded := calledBefore()
	changed := false
	if !seeded {
		for _, acc := range auth.AllAccounts() {
			if usage.TotalForOver(acc.ID, usage.Day, callerWindowDays) > 0 {
				known[acc.ID] = true
			}
		}
		data.SaveJSON(callersFile, known) //nolint:errcheck
		return
	}
	for _, acc := range auth.AllAccounts() {
		if known[acc.ID] {
			continue
		}
		// A generous window, because "first" here means the first time this
		// instance noticed rather than the first in history — and an instance
		// that was down for a day should not decide everybody is new.
		if usage.TotalForOver(acc.ID, usage.Day, callerWindowDays) == 0 {
			continue
		}
		known[acc.ID] = true
		changed = true
		raise(alert{
			Key:  "firstcall:" + acc.ID,
			What: acc.ID + " started using the API",
			Why: "Their first tool call. Signing up and using it are different things, " +
				"and this is the second one." + whatTheyDid(acc.ID),
			Where: "/admin/traffic",
		})
	}
	if changed {
		data.SaveJSON(callersFile, known) //nolint:errcheck
	}
}

// whatTheyDid is as much as can honestly be said about a first call.
//
// "Somebody started using the API" and nothing else leaves the operator to go
// and look, and going to look does not answer it either: internal/usage keeps
// counts per account and counts per tool and deliberately not the cross of the
// two — see SeriesFor, which says so and says why. So the counters cannot name
// the tool, and pretending otherwise here would mean inventing the key that
// package refuses to keep.
//
// What can be said is what they were charged for, because the ledger itemises
// every paid operation by name, and whether they have minted a token, because
// that is the step between signing up and calling anything. A free call — the
// weather, a listing, anything this instance stores itself — leaves no ledger
// row, and saying so is more useful than silence: it means they did something
// that cost us nothing, which is a different fact from doing nothing.
func whatTheyDid(accountID string) string {
	var out strings.Builder

	// What it cost, by operation, most recent first and deduplicated — five
	// calls to the same tool is one line, not five.
	seen := map[string]bool{}
	var ops []string
	for _, tx := range account.Transactions(accountID, 20) {
		if tx == nil || tx.Amount >= 0 || tx.Operation == "" || seen[tx.Operation] {
			continue
		}
		seen[tx.Operation] = true
		ops = append(ops, tx.Operation)
		if len(ops) == 5 {
			break
		}
	}
	if len(ops) > 0 {
		out.WriteString(" They have been charged for: " + strings.Join(ops, ", ") + ".")
	} else {
		out.WriteString(" Nothing they did was charged for, so it was a free " +
			"operation — a listing, the weather, something this instance stores " +
			"itself. The counters cannot say which: usage is kept per account " +
			"and per tool, never both at once.")
	}

	// And whether they went through the front door or the API door.
	switch n := len(auth.ListTokens(accountID)); {
	case n == 0:
		out.WriteString(" No token minted, so this was the web or a signed-in session.")
	case n == 1:
		out.WriteString(" They have minted a token.")
	default:
		out.WriteString(fmt.Sprintf(" They have minted %d tokens.", n))
	}

	return out.String()
}

const (
	callersFile      = "alerted_callers.json"
	callerWindowDays = 30
)

// calledBefore is who we have already announced, and whether we have ever
// looked. The second half is the difference between "nobody has called" and
// "this instance has never run this check", which are the same empty map.
func calledBefore() (known map[string]bool, seeded bool) {
	known = map[string]bool{}
	err := data.LoadJSON(callersFile, &known)
	return known, err == nil
}

// diskFilling is the one that actually takes an instance down.
//
// Everything else here is information. A full disk is an outage: mail stops
// being accepted, the record stops being written, and the failure shows up as
// unrelated errors everywhere at once.
func diskFilling() {
	at := number(settings.Get("ALERT_DISK_PERCENT"), 85)
	if at <= 0 {
		return
	}
	used, total, percent := app.DiskUsage()
	if total == 0 || percent < float64(at) {
		return
	}
	raise(alert{
		Key:  "disk",
		What: fmt.Sprintf("Disk is %.0f%% full", percent),
		Why: fmt.Sprintf("%s of %s used, and the threshold is %d%% (ALERT_DISK_PERCENT). "+
			"A full disk is an outage rather than a slowdown: mail stops being accepted and "+
			"the record stops being written.", gigabytes(used), gigabytes(total), at),
		Where: "/admin/server",
	})
}

func gigabytes(b uint64) string {
	return strconv.FormatFloat(float64(b)/(1024*1024*1024), 'f', 1, 64) + "GB"
}

// raise sends an alert to every admin, unless it has just been sent.
func raise(a alert) {
	if !enabled() || !fired(a.Key) {
		return
	}
	admins := admins()
	if len(admins) == 0 {
		// Worth a line in the log rather than silence: an instance with
		// thresholds set and nobody to tell is a configuration somebody meant
		// to finish.
		app.Log("alert", "%s — but this instance has no admin to tell", a.What)
		return
	}
	app.Log("alert", "%s", a.What)

	// HTML, with an absolute link. Two faults with one symptom, both reported
	// as "links are not clickable in any of the emails I got":
	//
	// A bare URL in a text/plain body is linked by some mail clients and not
	// others, and the ones that do not leave you selecting and copying. And a
	// relative href in an email points at wherever the mail client thinks it
	// is, which is nowhere — links in mail are absolute or they are decoration.
	//
	// The anchor text is the URL itself rather than a word, so the plain-text
	// version that reaches internal/thread still carries the address: strip the
	// tags off "<a href=X>X</a>" and X survives, off "<a href=X>look</a>" and
	// it does not.
	body := "<p>" + html.EscapeString(a.Why) + "</p>"
	if a.Where != "" {
		link := strings.TrimSuffix(origin.Self(), "/") + a.Where
		body += `<p><a href="` + html.EscapeString(link) + `">` +
			html.EscapeString(link) + `</a></p>`
	}
	for _, id := range admins {
		if err := mail.DeliverHere(mail.Local{
			Display: "Mu",
			From:    "no-reply@" + mail.ConfiguredDomain(),
			To:      id,
			Subject: a.What,
			Body:    body,
		}); err != nil {
			app.Log("alert", "could not tell %s: %v", id, err)
		}
		push.Send(id, push.Notification{Title: a.What, Body: a.Why, URL: a.Where})
	}
}

// admins is who gets told, sorted so the order is the same every time.
func admins() []string {
	var out []string
	for _, acc := range auth.AllAccounts() {
		if acc.Admin {
			out = append(out, acc.ID)
		}
	}
	sort.Strings(out)
	return out
}

// fired reports whether an alert may be sent now, and records that it was.
//
// The cooldown is what makes a threshold safe to set aggressively: crossing one
// costs a message, not a message every five minutes until it is fixed. In
// memory, so a restart may repeat one — which is the right way round, since the
// failure that matters is a missed alert and not a duplicated one.
func fired(key string) bool {
	cool := time.Duration(number(settings.Get("ALERT_COOLDOWN_MINUTES"), 360)) * time.Minute
	firedMu.Lock()
	defer firedMu.Unlock()
	if at, seen := lastFired[key]; seen && time.Since(at) < cool {
		return false
	}
	lastFired[key] = time.Now()
	return true
}

var (
	firedMu   sync.Mutex
	lastFired = map[string]time.Time{}
)

// enabled is whether this instance tells anybody anything.
//
// On by default. An operator who does not want to be told sets ALERTS=off, and
// one who wants a particular check off sets its threshold to 0 — both of which
// are easier to discover than a switch you have to find before anything works.
func enabled() bool {
	switch strings.ToLower(strings.TrimSpace(settings.Get("ALERTS"))) {
	case "off", "false", "0", "no", "none", "disabled":
		return false
	}
	return true
}

// number reads a threshold, falling back when it is unset or nonsense.
//
// Zero is meaningful and is not the fallback: it is how an operator turns one
// check off, so it has to survive being read.
//
// It takes the value rather than the key, so every setting this package reads
// is a literal in the source. docs/config_test.go scans for that literal to
// hold the configuration page honest in both directions, and a key arriving as
// a variable is invisible to it — fairly, since it is invisible to grep too.
func number(value string, fallback int) int {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
