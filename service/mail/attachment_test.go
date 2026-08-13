package mail

// A DMARC report is not a message body, and it is not nothing either.
//
// The bytes used to live in Body, base64-encoded. The message view decoded
// them and drew a table; the inbox preview, the search index and mail_inbox
// got "UEsDBAoAAAAIAOlI/VyKomDL8AEAAMUEAAAt…" instead of a message. Taking
// them out of Body fixed those three and silently broke the fourth — the view
// had nothing left to decode, so a report that had rendered as a table showed
// only that a zip had arrived.
//
// One field was serving two readers that wanted opposite things. The body
// describes; the attachment is kept beside it. This holds both ends, because
// fixing either one alone is what happened the first time.

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

const dmarcXML = `<?xml version="1.0" encoding="UTF-8" ?>
<feedback>
  <report_metadata>
    <org_name>google.com</org_name>
    <email>noreply-dmarc-support@google.com</email>
    <report_id>1234567890</report_id>
  </report_metadata>
  <policy_published>
    <domain>micro.mu</domain>
    <p>none</p>
  </policy_published>
  <record>
    <row>
      <source_ip>203.0.113.10</source_ip>
      <count>2</count>
      <policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>pass</spf></policy_evaluated>
    </row>
    <identifiers><header_from>micro.mu</header_from></identifiers>
  </record>
</feedback>`

func zipped(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAStoredDMARCReportStillRendersAsATable(t *testing.T) {
	msg := &Message{
		FromID:         "noreply-dmarc-support@google.com",
		Body:           "[report.zip (application/zip), 4 KB — not shown]",
		Attachment:     base64.StdEncoding.EncodeToString(zipped(t, "report.xml", dmarcXML)),
		AttachmentType: "application/zip",
		AttachmentName: "report.zip",
	}

	html, name := renderStoredAttachment(msg)
	if html == "" {
		t.Fatal("a stored DMARC report renders nothing — the inbox shows only that a zip arrived")
	}
	for _, want := range []string{"google.com", "micro.mu", "203.0.113.10"} {
		if !strings.Contains(html, want) {
			t.Errorf("the rendered report does not mention %q:\n%s", want, html)
		}
	}
	if !strings.Contains(html, "<table") {
		t.Error("the report is not rendered as a table")
	}
	if name != "report.zip" {
		t.Errorf("attachment name is %q, want report.zip", name)
	}
}

// Bare XML and gzip are the other two shapes a report arrives in.
func TestAReportRendersWhateverItIsWrappedIn(t *testing.T) {
	for _, c := range []struct {
		name  string
		bytes []byte
	}{
		{"bare xml", []byte(dmarcXML)},
		{"zip", zipped(t, "report.xml", dmarcXML)},
	} {
		msg := &Message{
			FromID:     "dmarc@example.com",
			Attachment: base64.StdEncoding.EncodeToString(c.bytes),
		}
		if html, _ := renderStoredAttachment(msg); html == "" {
			t.Errorf("%s: rendered nothing", c.name)
		}
	}
}

// And the half that was fixed first must stay fixed: nothing binary goes into
// the body, where it would reach the preview, the index and mail_inbox.
func TestTheBodyNeverCarriesTheAttachment(t *testing.T) {
	raw := zipped(t, "report.xml", dmarcXML)
	body := describeAttachment("report.zip", "application/zip", len(raw))

	if strings.Contains(body, "UEsDBA") || looksLikeBase64(body) {
		t.Errorf("the body carries encoded bytes again: %q", body)
	}
	if !strings.Contains(body, "report.zip") {
		t.Errorf("the body does not say what arrived: %q", body)
	}
	// Short enough that a card preview is a sentence, not a wall.
	if len(body) > 120 {
		t.Errorf("the description is %d characters — too long for a preview: %q", len(body), body)
	}
}

// Nothing to render is not an error, and must not blank a real body.
func TestAMessageWithNoAttachmentRendersNothing(t *testing.T) {
	for _, msg := range []*Message{
		nil,
		{Body: "hello"},
		{Body: "hello", Attachment: "not base64 !!!"},
		{Body: "hello", Attachment: base64.StdEncoding.EncodeToString([]byte("plain text, no report"))},
	} {
		if html, _ := renderStoredAttachment(msg); html != "" {
			t.Errorf("rendered %q for a message with no report", html)
		}
	}
}

// A report that arrives as one part keeps its bytes.
//
// This is the half that was still broken. The multipart path handed its
// attachment back so the message view could render the table; the single-part
// path described the attachment and dropped it — and Google sends aggregate
// reports as a single application/zip part with no text alongside. So the body
// read "[google.com!….zip (application/zip), 707 bytes — not shown]" and there
// was nothing behind it, for months, looking exactly like a renderer that had
// stopped working.
func TestASinglePartReportKeepsItsBytes(t *testing.T) {
	raw := zipped(t, "google.com!micro.mu!1786492800!1786579199.xml", dmarcXML)

	body, att := singlePartBody(raw, `application/zip; name="google.com!micro.mu!1.zip"`)

	if att == nil {
		t.Fatal("the bytes were dropped, so the message view has nothing to render")
	}
	if !bytes.Equal(att.Content, raw) {
		t.Error("the stored bytes are not the ones that arrived")
	}
	if !strings.Contains(body, "not shown") {
		t.Errorf("the body does not say what arrived: %q", body)
	}
	// And nothing binary reached the body, which is the rule that came first.
	if strings.Contains(body, "PK") {
		t.Errorf("the zip leaked into the body: %q", body)
	}

	// End to end: what was kept is what renders.
	stored := &Message{
		FromID:         "noreply-dmarc-support@google.com",
		Body:           body,
		Attachment:     base64.StdEncoding.EncodeToString(att.Content),
		AttachmentType: att.Type,
		AttachmentName: att.Name,
	}
	html, _ := renderStoredAttachment(stored)
	if !strings.Contains(html, "<table") {
		t.Errorf("a single-part report still does not render as a table:\n%s", html)
	}
}

// Text is still just the body — nothing is stored beside a plain message.
func TestAPlainSinglePartMessageStoresNoAttachment(t *testing.T) {
	body, att := singlePartBody([]byte("hello there"), "text/plain; charset=utf-8")
	if att != nil {
		t.Error("a plain text message was filed as carrying an attachment")
	}
	if body != "hello there" {
		t.Errorf("body = %q", body)
	}
}
