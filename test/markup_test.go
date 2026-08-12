package test

// No Go identifier should reach a reader.
//
// The login page said "app.Log in". So did the signup page, twice: "app.Log in
// if you have an account". A rename of Log to app.Log had been applied to the
// token wherever it appeared, including inside the HTML, and the result sat in
// the heading of the first page a new visitor sees.
//
// Nothing caught it because it compiles, renders and is the right length. Only
// a person reading the page would notice, and the people who work on it stopped
// reading that heading a long time ago.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// qualified matches a package-qualified Go name sitting in text a reader sees:
// between a > and a < , or at the start of a line of markup.
var qualified = regexp.MustCompile(`>\s*(app|auth|wallet|home|service|quota|data|api)\.[A-Z][a-zA-Z]*`)

// TestNoGoIdentifierIsRenderedToTheReader.
func TestNoGoIdentifierIsRenderedToTheReader(t *testing.T) {
	root := at("")
	var found []string

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error { //nolint:errcheck
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(b), "\n") {
			m := qualified.FindString(line)
			if m == "" {
				continue
			}
			// A call or a concatenation is code, not text: `>` + app.Link(...)
			// and `>` + app.RenderString(x) are how half these pages are built.
			// What is being looked for is the identifier as prose, which means
			// no ( and no + between the > and the name.
			between := line[strings.Index(line, m):]
			if strings.ContainsAny(firstToken(between), "(+`\"") {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			found = append(found, rel+":"+itoa(i+1)+"  "+strings.TrimSpace(line))
		}
		return nil
	})

	for _, f := range found {
		t.Errorf("a Go identifier is rendered as text — %s", f)
	}
}

// firstToken is the identifier and whatever immediately follows it, which is
// what says whether this is a call or a word on a page.
func firstToken(s string) string {
	s = strings.TrimPrefix(s, ">")
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '<' {
			return s[:i]
		}
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
