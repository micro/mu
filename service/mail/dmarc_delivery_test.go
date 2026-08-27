package mail

// The whole way a DMARC report travels, from the wire to the table.
//
// Each half was tested and the join was not, which is how the report kept
// arriving as a line of text saying a zip had come. The parser had tests
// proving it kept bytes out of the body; the renderer had tests proving it drew
// a table from bytes. Nothing put the two together, so a path where the parser
// dropped the bytes looked exactly like a renderer that had stopped working.

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

// googleReport is the shape Google actually sends: multipart/mixed, a short
// text part, and the report zipped and base64'd beside it.
func googleReport(t *testing.T, boundary string, withText bool) string {
	t.Helper()
	zip := base64.StdEncoding.EncodeToString(
		zipped(t, "google.com!micro.mu!1786492800!1786579199.xml", dmarcXML))

	var b strings.Builder
	if withText {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/plain; charset=us-ascii\r\n\r\n")
		b.WriteString("This is an aggregate report from google.com.\r\n")
	}
	b.WriteString("--" + boundary + "\r\n")
	b.WriteString(`Content-Type: application/zip; name="google.com!micro.mu!1786492800!1786579199.zip"` + "\r\n")
	b.WriteString(`Content-Disposition: attachment; filename="google.com!micro.mu!1786492800!1786579199.zip"` + "\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(zip + "\r\n")
	b.WriteString("--" + boundary + "--\r\n")
	return b.String()
}

// A report with a covering note beside it: the note is the body, the zip is
// kept, and the view renders the table from what was kept.
func TestAMultipartReportArrivesWithItsBytesAndRenders(t *testing.T) {
	const boundary = "sep123"
	body, att := parseMultipart(strings.NewReader(googleReport(t, boundary, true)), boundary)

	if att == nil {
		t.Fatal("the zip was dropped, so the view has nothing to render")
	}
	if !strings.Contains(body, "aggregate report") {
		t.Errorf("the covering note is not the body: %q", body)
	}
	if strings.Contains(body, "PK") {
		t.Errorf("the zip leaked into the body: %q", body)
	}

	assertRenders(t, body, att)
}

// And one with no note at all, which is the case that produced the line of text
// somebody kept seeing.
func TestAReportWithNoTextPartStillRenders(t *testing.T) {
	const boundary = "sep456"
	body, att := parseMultipart(strings.NewReader(googleReport(t, boundary, false)), boundary)

	if att == nil {
		t.Fatal("the zip was dropped, so the view has nothing to render")
	}
	if !strings.Contains(body, "not shown") {
		t.Errorf("with no text part the body should say what arrived: %q", body)
	}
	assertRenders(t, body, att)
}

// assertRenders files the message the way delivery does and checks the view
// draws the table.
func assertRenders(t *testing.T, body string, att *Attachment) {
	t.Helper()
	msg := &Message{
		From:           "noreply-dmarc-support@google.com",
		FromID:         "noreply-dmarc-support@google.com",
		Body:           body,
		Attachment:     base64.StdEncoding.EncodeToString(att.Content),
		AttachmentType: att.Type,
		AttachmentName: att.Name,
	}
	html, _ := renderStoredAttachment(msg)
	if html == "" {
		t.Fatal("the stored report renders nothing")
	}
	for _, want := range []string{"<table", "google.com", "203.0.113.10"} {
		if !strings.Contains(html, want) {
			t.Errorf("the table is missing %q:\n%s", want, html)
		}
	}
	// And the message would not be mistaken for one whose bytes were lost.
	if describedNothing(msg) {
		t.Error("a message that kept its report is reported as having lost it")
	}
}

// The trust check is on the sender, and it is what stops a stranger's zip being
// unpacked. Google's reporting address is the one that has to pass it.
func TestGooglesReportingAddressIsTrustedToUnpack(t *testing.T) {
	raw := zipped(t, "report.xml", dmarcXML)

	for _, from := range []string{
		"noreply-dmarc-support@google.com",
		"dmarcreport@microsoft.com",
		"dmarc@yahoo.com",
	} {
		if extractZipContents(raw, from) == "" {
			t.Errorf("%s is not trusted to have its report unpacked", from)
		}
	}
	if extractZipContents(raw, "someone@example.com") != "" {
		t.Error("a stranger's zip was unpacked")
	}
}

// There is one place that renders a message body, and it reads the attachment.
//
// This test used to assert something weaker — that *each* of the two renderers
// read the bytes stored beside the body — because there were two: one for the
// message being viewed and one inside the loop that renders a thread. Only one
// of them had been taught, so a DMARC report opened in a thread printed the
// line describing the zip and nothing else. Worse, it could not report its own
// failure: the note about bytes that were never kept lived in the other copy,
// so "never stored" and "stored but ignored" looked identical from the page.
//
// The copies are gone — see Rendered in display.go — and the invariant is now
// the stronger one. Not "every renderer must be taught" but "there is one
// renderer to teach", which is what stops the next page from being wrong in a
// new way. It is also what let /inbox be wrong for months: with the decoding
// inline in an HTTP handler there was nothing for a second page to call, so it
// escaped the text instead, and an escaped table is not a table.
func TestThereIsOneBodyRenderer(t *testing.T) {
	var renders, reads, reports int
	for _, name := range sourceFiles(t) {
		src := readSource(t, name)
		// Calls, not declarations: "func renderEmailBody(" is the renderer
		// itself and counting it would make one caller look like two.
		renders += calls(src, "renderEmailBody(")
		reads += calls(src, "renderStoredAttachment(")
		reports += calls(src, "describedNothing(")
	}
	// One call, in Rendered. Its own definition is not a call.
	if renders != 1 {
		t.Errorf("%d places render a message body; there must be exactly one, or "+
			"two pages showing the same message can disagree and no caller "+
			"outside this package can render one at all", renders)
	}
	if reads < renders {
		t.Errorf("%d renderers and %d read the attachment stored beside the body — "+
			"one shows the line describing a report instead of the report",
			renders, reads)
	}
	if reports < renders {
		t.Errorf("a body renderer cannot tell a lost attachment from a broken one, "+
			"which is what let this run: %d renderers, %d checks", renders, reports)
	}
}

// calls counts uses of a function, ignoring the line that declares it.
func calls(src, name string) int {
	return strings.Count(src, name) - strings.Count(src, "func "+name)
}

// sourceFiles is this package's own Go files, tests excluded.
//
// The scan used to name mail.go and nothing else, which is precisely how a copy
// in a second file would go unseen — the same shape as the rule this test is
// about.
func sourceFiles(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range names {
		if !strings.HasSuffix(n, "_test.go") {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		t.Fatal("no source files found, so this test is checking nothing")
	}
	return out
}
