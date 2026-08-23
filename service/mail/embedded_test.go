package mail

// A part we cannot render does not get pasted into the body.
//
// message/rfc822 is the one that broke it: a bounce or a DMARC failure report
// embeds the original message, and its *content is a MIME entity* — From,
// Subject, Content-Disposition, a blank line and a block of base64. All of that
// went into the body verbatim, so an IMAP client showed raw headers and base64
// where the message should have been, and so did the inbox list, the search
// index and anything an agent read through mail_inbox.
//
// The rule was already written down a few lines away, about attachments: if we
// cannot render it, we do not paste it in — say what arrived.

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAnEmbeddedMessageIsDescribedNotInlined(t *testing.T) {
	raw := "--BB\r\n" +
		"Content-Type: text/plain\r\n\r\nSee the attached report.\r\n" +
		"--BB\r\n" +
		"Content-Type: message/rfc822\r\n\r\n" +
		"From: sender@example.com\r\n" +
		"Subject: the original\r\n" +
		"Content-Disposition: inline\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString([]byte("the original body")) + "\r\n" +
		"--BB--\r\n"

	body, _ := parseMultipart(strings.NewReader(raw), "BB")

	if !strings.Contains(body, "See the attached report.") {
		t.Errorf("the readable part was lost:\n%s", body)
	}
	for _, leak := range []string{
		"Content-Disposition", "Content-Transfer-Encoding", "From: sender@example.com",
	} {
		if strings.Contains(body, leak) {
			t.Errorf("raw MIME reached the body (%q):\n%s", leak, body)
		}
	}
	// Said, rather than silently dropped: something did arrive.
	if !strings.Contains(body, "message/rfc822") {
		t.Errorf("the embedded message is not mentioned at all:\n%s", body)
	}
}

// A text part of some other kind is legible and is kept. The point was never to
// throw parts away — it was to stop pasting bytes in.
func TestOtherTextPartsAreKept(t *testing.T) {
	raw := "--BB\r\n" +
		"Content-Type: text/plain\r\n\r\nLunch?\r\n" +
		"--BB\r\n" +
		"Content-Type: text/calendar; method=REQUEST\r\n\r\n" +
		"BEGIN:VCALENDAR\r\nSUMMARY:Lunch\r\nEND:VCALENDAR\r\n" +
		"--BB--\r\n"

	body, _ := parseMultipart(strings.NewReader(raw), "BB")
	if !strings.Contains(body, "BEGIN:VCALENDAR") || !strings.Contains(body, "SUMMARY:Lunch") {
		t.Errorf("a readable calendar part was dropped:\n%s", body)
	}
}

// The ordinary DMARC report still reads as it should: the explainer is the
// body and the zip is an attachment, not a wall of base64.
func TestADmarcReportReadsAsItsExplainer(t *testing.T) {
	zip := base64.StdEncoding.EncodeToString([]byte("PK\x03\x04not-really-a-zip"))
	raw := "--BB\r\n" +
		"Content-Type: text/plain; charset=us-ascii\r\n\r\n" +
		"This is an aggregate report from google.com.\r\n" +
		"--BB\r\n" +
		"Content-Type: application/zip\r\n" +
		"Content-Disposition: attachment; filename=\"google.com!micro.mu!1.zip\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" + zip + "\r\n" +
		"--BB--\r\n"

	body, att := parseMultipart(strings.NewReader(raw), "BB")
	if strings.TrimSpace(body) != "This is an aggregate report from google.com." {
		t.Errorf("the body is not the explainer: %q", body)
	}
	if att == nil || att.Name != "google.com!micro.mu!1.zip" {
		t.Errorf("the zip was not kept as an attachment: %+v", att)
	}
	if strings.Contains(body, zip[:20]) {
		t.Error("the zip was pasted into the body")
	}
}
