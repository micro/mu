package mail

import (
	"regexp"
	"strings"
	"testing"
)

// A value cannot become a header.
//
// Everything reachable here is written by somebody: the agent composes mail
// from web pages and inbound messages it was asked to act on, a display name is
// whatever an account typed into its profile, /inbox is a text box, and a mail
// client sets its own From. A newline in any of them used to add a header, and
// the header worth adding is Bcc.
func TestNoValueCanAddAHeader(t *testing.T) {
	attacks := []struct {
		name string
		in   string
	}{
		{"CRLF, the textbook one", "Invoice\r\nBcc: somebody@else.example"},
		{"a bare LF, which some servers split on", "Invoice\nBcc: somebody@else.example"},
		{"a bare CR, which others do", "Invoice\rBcc: somebody@else.example"},
		{"a folded continuation", "Invoice\r\n\tBcc: somebody@else.example"},
		{"two of them", "a\r\nX-One: 1\r\nX-Two: 2"},
		{"a NUL, which truncates at the other end of the wire", "Invoice\x00Bcc: x@y.example"},
		{"a vertical tab", "Invoice\v Bcc: x@y.example"},
		{"a form feed", "Invoice\f Bcc: x@y.example"},
	}

	for _, a := range attacks {
		t.Run(a.name, func(t *testing.T) {
			for _, got := range []string{
				headerText(a.in),
				headerAddr(a.in),
				headerFrom(a.in, "someone@example.test"),
				headerFrom("Someone", a.in),
			} {
				if strings.ContainsAny(got, "\r\n") {
					t.Errorf("a line ending survived: %q", got)
				}
				if strings.Contains(got, "\x00") {
					t.Errorf("a NUL survived: %q", got)
				}
			}
		})
	}
}

// The encoding is only reached for values that need it, and a header that needs
// it is not left as raw bytes.
func TestHeaderTextEncodesWhatIsNotASCII(t *testing.T) {
	if got := headerText("Plain subject"); got != "Plain subject" {
		t.Errorf("an ASCII subject was rewritten to %q", got)
	}
	got := headerText("Rechnung über 100 €")
	if strings.Contains(got, "ü") || strings.Contains(got, "€") {
		t.Errorf("raw UTF-8 went into a header: %q — a raw UTF-8 byte in a "+
			"header is not a header", got)
	}
	if !strings.HasPrefix(got, "=?utf-8?") {
		t.Errorf("%q is not an encoded word, so a client will show mojibake", got)
	}
}

// An address header keeps its syntax. RFC 2047 encoding an address list would
// produce an encoded-word where an address belongs, which is not an address.
func TestHeaderAddrDoesNotEncodeAddresses(t *testing.T) {
	const list = "one@example.test, two@example.test"
	if got := headerAddr(list); got != list {
		t.Errorf("an address list was rewritten to %q", got)
	}
	const chain = "<a@example.test> <b@example.test>"
	if got := headerAddr(chain); got != chain {
		t.Errorf("a References chain was rewritten to %q", got)
	}
}

// And the same, asserted on the bytes that actually go on the wire — because
// the helpers being right is not the same as the builders calling them.
//
// There were two builders and each wrote its own From, To and Subject with
// Sprintf. Fixing one of them is the failure mode this whole week has been
// about.
func TestNeitherMessageBuilderTakesAValueRaw(t *testing.T) {
	src := readSource(t, "client.go")

	// Every write, not merely one of them. Contains() was the first shape of
	// this check and it passed with one builder reverted, because the other
	// builder's correct line satisfied it — which is the failure mode the test
	// exists to catch, reproduced inside the test.
	writes := regexp.MustCompile(`fmt\.Sprintf\("([A-Za-z-]+): %s\\r\\n", ([^)]*)\)`)
	found := map[string]int{}
	for _, m := range writes.FindAllStringSubmatch(src, -1) {
		header, arg := m[1], strings.TrimSpace(m[2])
		found[header]++
		// Built here from a timestamp and this instance's own domain, so there
		// is nothing from outside in either of them to fold.
		if header == "Message-ID" || header == "Date" {
			continue
		}
		if !strings.HasPrefix(arg, "header") {
			t.Errorf("%s is written from %s, which does not go through header.go — "+
				"a value with a newline in it adds a header rather than filling one",
				header, arg)
		}
	}

	// The regex has to have seen something, or this passes by matching nothing.
	for _, header := range []string{"From", "To", "Subject"} {
		if found[header] < 2 {
			t.Errorf("only found %d %s writes; there are two message builders and "+
				"this check has stopped seeing one of them", found[header], header)
		}
	}

	// And nothing rebuilds a From by hand, which is how the display name got in
	// raw on both of them.
	if strings.Contains(src, `fmt.Sprintf("%s <%s>", displayName, from)`) {
		t.Error("a From header is still assembled by hand, so a display name of " +
			`"Bob\r\nBcc: ..." forges a header`)
	}
}
