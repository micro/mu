package test

// A missing key is not an empty result.
//
// The two look identical to a caller and only one of them is worth waiting for.
// An agent told "no videos available right now" reports that there is nothing
// to say and moves on; an agent told the instance has no YouTube key can tell
// its owner to go and set one. The operator, meanwhile, never finds out either
// way — the instance looks like it is working and quietly does half of what it
// claims.
//
// service/web got this right and carried the reasoning in a comment — "'
// unavailable right now' tells them to wait for something that is never coming"
// — and was the only service that had it. Every other key-gated service
// reported a missing key as an empty result, for months, next to a file
// explaining why that was wrong.
//
// This is the rule rather than a list, so the next service gated on a key
// either follows it or fails here.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// keyGated is every service whose usefulness depends on a credential the
// operator has to supply, and the variable that supplies it.
//
// Deliberately hand-written. A service is added here when somebody decides it
// needs a key, which is the moment to decide what it says without one.
var keyGated = map[string]string{
	"web":     "BRAVE_API_KEY",
	"video":   "YOUTUBE_API_KEY",
	"sms":     "TWILIO",
	"transit": "BODS_API_KEY",
}

// Words that mean "this instance cannot do this until somebody configures it",
// as opposed to "there is nothing to report".
var saysItIsConfiguration = []string{
	"not configured",
	"no number to send from",
	"an admin can set",
	"The operator needs to",
	"operator can set",
	"has no",
}

func TestAKeyGatedServiceSaysWhenItHasNoKey(t *testing.T) {
	for svc, envVar := range keyGated {
		dir := filepath.Join("..", "service", svc)
		src, err := readPackage(dir)
		if err != nil {
			t.Errorf("%s: %v", svc, err)
			continue
		}

		// It has to know. A service that cannot tell whether it is configured
		// cannot say so, which is how the empty-result answer happens: there
		// is nothing to branch on.
		if !strings.Contains(src, "Configured") {
			t.Errorf("service/%s is gated on %s and has no Configured() to ask, so "+
				"an unconfigured instance cannot report itself as one", svc, envVar)
		}

		// And it has to say it in words a person can act on.
		said := false
		for _, phrase := range saysItIsConfiguration {
			if strings.Contains(src, phrase) {
				said = true
				break
			}
		}
		if !said {
			t.Errorf("service/%s never tells a caller that a missing %s is the "+
				"reason it returned nothing — so an agent reads a missing key as "+
				"an empty result and the operator is never told", svc, envVar)
		}
	}
}

// readPackage concatenates a package's non-test sources.
func readPackage(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		by, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		b.Write(by)
	}
	return b.String(), nil
}
