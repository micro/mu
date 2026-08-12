package test

// A page that sends somebody to /admin/env has to be right about it.
//
// /sms lists what is missing and says "an operator sets these at /admin/env".
// /whatsapp names TWILIO_WHATSAPP_FROM and does the same. Neither variable was
// on that page — every Twilio setting was absent from it — so the instruction
// was a dead end, and the only way to find that out was to follow it.
//
// The settings the admin page offers are a hand-written list, which is the same
// shape as every other drift found in this repo: a list beside the thing it
// describes, with nothing holding the two together. This holds them together.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"mu/admin"
	"mu/service/sms"
)

// TestWhatSMSAsksForCanBeSetWhereItSaysToSetIt.
func TestWhatSMSAsksForCanBeSetWhereItSaysToSetIt(t *testing.T) {
	// Missing() names the variables in prose — "TWILIO_ACCOUNT_SID — the
	// account, which starts with A then C" — so take the name off the front.
	for _, line := range sms.Missing() {
		for _, name := range namesIn(line) {
			if !admin.Settable(name) {
				t.Errorf("/sms tells an operator to set %s at /admin/env, and it is not there", name)
			}
		}
	}
}

// TestEveryVariableAPageNamesIsSettable scans for a variable named in the same
// breath as /admin/env and checks the page can actually set it.
func TestEveryVariableAPageNamesIsSettable(t *testing.T) {
	// Read by the code but deliberately not offered: a secret that must exist
	// before the store it protects can be opened, addresses fixed at start-up,
	// and one proof file. Setting any of these from a page inside the running
	// process either cannot work or defeats the point.
	notSettable := map[string]bool{
		"MU_ENCRYPTION_KEY":  true, // unlocks the store the setting would be saved in
		"MCP_GATEWAY_ADDR":   true, // a listen address, bound at boot
		"MCP_REGISTRY_PROOF": true,
		"APP_URL":            true,
		"PUBLIC_URL":         true,
		// Names the first admin at boot. It cannot live on the page it is the
		// way in to: an instance with no admin has nobody who can open it.
		"ADMIN": true,
	}

	pattern := regexp.MustCompile(`([A-Z][A-Z0-9_]{3,})`)
	root := at("")
	var problems []string

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/admin/") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.Contains(line, "/admin/env") {
				continue
			}
			for _, m := range pattern.FindAllString(line, -1) {
				if notSettable[m] || admin.Settable(m) {
					continue
				}
				// Only names the code actually reads as settings, so prose like
				// TODO or an HTTP verb in the same line is not mistaken for one.
				if !readsAsSetting(root, m) {
					continue
				}
				rel, _ := filepath.Rel(root, path)
				problems = append(problems, rel+" points at /admin/env for "+m)
			}
		}
		return nil
	})

	for _, p := range problems {
		t.Errorf("%s — but it cannot be set there", p)
	}
}

// namesIn pulls SHOUTING_CASE identifiers out of a sentence.
func namesIn(s string) []string {
	return regexp.MustCompile(`\b[A-Z][A-Z0-9_]{3,}\b`).FindAllString(s, -1)
}

// readsAsSetting reports whether the tree reads this name through settings.Get,
// which is what makes it a setting rather than a word in capitals.
func readsAsSetting(root, name string) bool {
	found := false
	re := regexp.MustCompile(`(?:settings\.Get|os\.Getenv)\("` + regexp.QuoteMeta(name) + `"\)`)
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if found || err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err == nil && re.Match(b) {
			found = true
		}
		return nil
	})
	return found
}

// TestTheOperatorCanTurnTheNetworkWatcherOn — SOCIAL_ATPROTO is the one setting
// whose whole design is "off until an operator decides", and it was documented
// as living on a page that did not offer it.
func TestTheOperatorCanTurnTheNetworkWatcherOn(t *testing.T) {
	if !admin.Settable("SOCIAL_ATPROTO") {
		t.Error("watching the open network cannot be turned on from /admin/env, " +
			"which is where INSTALL.md says to turn it on")
	}
}
