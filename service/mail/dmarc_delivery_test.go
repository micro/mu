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

// Every place that renders a message body has to read the stored attachment.
//
// There are two, and only one of them knew. The single-message view was taught
// to read bytes kept beside the body when they moved out of it; the thread loop
// — which is how a conversation is shown, including a conversation of one — was
// not, and still sniffs m.Body for base64 and zip magic because that is where
// attachments used to live. So a DMARC report opened in a thread printed the
// line describing the zip and nothing else.
//
// Worse, it could not report its own failure: the note about bytes that were
// never kept lived in the other block, so the two states — never stored, and
// stored but ignored — looked identical from the page. Two renderers, one
// taught and one not, is the shape of this bug rather than a detail of it.
func TestEveryBodyRendererReadsTheStoredAttachment(t *testing.T) {
	src := readSource(t, "mail.go")

	renders := strings.Count(src, "renderEmailBody(")
	reads := strings.Count(src, "renderStoredAttachment(")
	if renders == 0 {
		t.Fatal("no body renderer found, so this test is checking nothing")
	}
	if reads < renders {
		t.Errorf("%d places render a message body and only %d read the attachment "+
			"stored beside it — one of them shows the line describing a report "+
			"instead of the report", renders, reads)
	}
	// And each of them says so when the bytes are gone, rather than leaving a
	// description with nothing behind it and no explanation.
	if strings.Count(src, "describedNothing(") < renders {
		t.Errorf("a body renderer cannot tell a lost attachment from a broken one, "+
			"which is what let this run: %d renderers, %d checks",
			renders, strings.Count(src, "describedNothing("))
	}
}
