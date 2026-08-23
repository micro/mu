package test

// A link in an email is absolute, or it is decoration.
//
// Reported as "links are not clickable in any of the emails I got". Half of it
// was that a bare URL in a text/plain body is linked by some clients and not
// others; the other half was worse and silent — the notices this instance
// sends carried href="/tools", and a relative href in an email resolves
// against wherever the mail client thinks it is. It goes nowhere, and it looks
// like a link while doing it.
//
// The pages have the opposite rule: everything internal is relative, because
// the instance does not know its own public name on every deployment and a
// hard-coded host is how a self-hosted copy ends up linking to micro.mu. So
// this cannot be a rule about the codebase — it is a rule about the files that
// write mail, and they are named here.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// writesMail is every file that composes a message body. Short on purpose: if
// it grows, that is a signal that composing mail has spread out, which is its
// own problem.
var writesMail = []string{
	"../admin/alert.go",
	"../agent/mail/welcome.go",
}

var relativeHref = regexp.MustCompile(`href=\\?"/`)

func TestNothingSendsMailWithARelativeLink(t *testing.T) {
	for _, path := range writesMail {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v — if this file moved, move the rule with it", path, err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			if relativeHref.MatchString(line) {
				t.Errorf(`%s:%d writes a relative link into a message. In mail that `+
					`resolves against the mail client and goes nowhere — build it `+
					`from origin.Self():`+"\n\t%s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
