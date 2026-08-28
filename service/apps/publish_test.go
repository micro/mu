package apps

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Publish takes a directory, never a document.
//
// This is the whole point of it, and it is a shape that is easy to lose: the
// obvious "improvement" is to let a caller pass the HTML directly so the
// endpoint works without a machine. That would put the page back into the
// caller's context, which is the thing this exists to avoid — a five-kilobyte
// page passed as a tool argument is what made a live model emit its own
// tool-call markup as text and create nothing.
//
// So the request having no content field is the feature, and this fails if
// somebody adds one.
func TestPublishTakesADirectoryNotADocument(t *testing.T) {
	b, err := os.ReadFile("publish.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	start := strings.Index(src, "type PublishRequest struct")
	if start < 0 {
		t.Fatal("PublishRequest is gone")
	}
	end := strings.Index(src[start:], "}")
	if end < 0 {
		t.Fatal("PublishRequest does not end")
	}
	req := src[start : start+end]

	// gofmt aligns struct fields, so the gap between a name and its type is
	// whatever keeps the block tidy — matching one space would pass whatever
	// was written.
	field2type := func(name string) *regexp.Regexp {
		return regexp.MustCompile(`\b` + name + `\s+string\b`)
	}
	for _, field := range []string{"HTML", "Html", "Content", "Body", "Source"} {
		if field2type(field).MatchString(req) {
			t.Errorf("PublishRequest carries %s. The page must come from the "+
				"machine, not from the caller — passing it as an argument is "+
				"what breaks on anything bigger than a toy:\n%s", field, req)
		}
	}
	if !field2type("Dir").MatchString(req) {
		t.Errorf("PublishRequest no longer takes a directory:\n%s", req)
	}
}

// The scanner runs before anything is hosted.
//
// Every other route into CreateApp carries HTML this instance asked a model
// for. This one carries whatever was lying in a directory that had a shell and
// a network in it, so the scan is not a formality here — it is the only thing
// between a file an agent wrote and a page this instance serves on its own
// domain. A publish that skipped it would be the hole the other doors are
// careful not to be.
func TestPublishScansBeforeHosting(t *testing.T) {
	b, err := os.ReadFile("publish.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)

	scan := strings.Index(src, "ScanApp(")
	if scan < 0 {
		t.Fatal("Publish does not scan what it is about to host")
	}
	// And it scans before it saves, which is the half that matters: a scan
	// after CreateApp is a report, not a refusal.
	for _, save := range []string{"CreateApp(", "UpdateAppOwned("} {
		at := strings.Index(src, save)
		if at < 0 {
			t.Fatalf("Publish no longer calls %s", save)
		}
		if at < scan {
			t.Errorf("%s runs before ScanApp, so a refused page is hosted first "+
				"and judged afterwards", save)
		}
	}
}

// Publishing the same slug twice changes the app rather than refusing.
//
// This is how a page is iterated on: edit the file in the box, publish again.
// If the second publish errored, every change would need a new slug and the
// app somebody linked to would be the first draft forever.
func TestPublishingAgainReplacesTheApp(t *testing.T) {
	b, err := os.ReadFile("publish.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if !strings.Contains(src, "UpdateAppOwned(") {
		t.Error("publishing to a slug that exists does not update it, so an app " +
			"can be created but never changed from the machine it was built on")
	}
	// Ownership is decided by UpdateAppOwned, not re-derived here. Two places
	// deciding who owns an app is one place too many.
	if strings.Contains(src, "AuthorID ==") || strings.Contains(src, "AuthorID !=") {
		t.Error("Publish checks ownership itself instead of leaving it to " +
			"UpdateAppOwned, which is where that rule lives")
	}
}
