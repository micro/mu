package inbox

import (
	"strings"
	"testing"

	"mu/internal/thread"
	"mu/service/mail"
)

// A mail message reads in a thread the way it reads in the mail client.
//
// This is the bug that kept coming back. A DMARC report *is* a table, and the
// record holds the prose version of a message — correctly, because prose is
// what an agent should be handed and what a search should look through. This
// page was rendering that prose with html.EscapeString, so a table arrived as
// the word "table" and some angle brackets, while /mail showed it properly.
//
// Two pages, two renderings, one message. The fix is not a second renderer
// here: it is that the mail service has exactly one (mail.Rendered) and this
// asks for it.
func TestAMailMessageRendersTheSameInAThread(t *testing.T) {
	const who = "inbox_render"
	const msgID = "<report-1@dmarc.example>"

	// Stored the way a real report is stored: the description in the body, the
	// bytes beside it. That split is deliberate — the body is what an agent
	// reads and what the index searches, so the report itself is kept apart —
	// and it is exactly what makes the record's copy unrenderable on its own.
	//
	// google.com because unpacking somebody's archive is gated on the sender
	// being a known reporter. A stranger's zip is not opened, which is a rule
	// worth not breaking to make a test pass.
	if err := mail.SendMessageTo(mail.Delivery{
		From:      "noreply-dmarc-support@google.com",
		FromID:    "noreply-dmarc-support@google.com",
		To:        who,
		ToID:      who,
		Subject:   "Report Domain: example.com",
		Body:      "[dmarc-report.xml: 1 attachment]",
		MessageID: msgID,
		Attachment: &mail.Attachment{
			Name:    "dmarc-report.xml",
			Type:    "text/xml",
			Content: []byte(dmarcReport),
		},
	}); err != nil {
		t.Fatalf("store the message: %v", err)
	}

	// Recorded the way agent/mail records it: prose, joined to the stored
	// message by the Message-ID in Ref.
	th := thread.Open(who, "mail", msgID)
	if th == nil {
		t.Fatal("could not open the conversation")
	}
	t.Cleanup(func() { thread.Forget(who) })
	thread.Name(who, th.ID, "Report Domain: example.com")
	thread.Add(thread.Message{
		Thread: th.ID, Account: who, Text: "[dmarc-report.xml: 1 attachment]",
		Ref: msgID, From: "noreply-dmarc-support@google.com", To: who,
	})

	out := ConversationView(who, th)

	if !strings.Contains(out, "<table") {
		t.Errorf("the report rendered as text rather than a table — this is the "+
			"complaint, in one assertion:\n%s", clip(out))
	}
	if strings.Contains(out, "&lt;table") {
		t.Error("the markup was escaped and shown as characters, which is what " +
			"html.EscapeString on a mail body does")
	}
	// And the reader gets the report rather than the line describing it.
	if strings.Contains(out, "1 attachment") && !strings.Contains(out, "<table") {
		t.Error("the page shows the description of the attachment instead of the " +
			"attachment")
	}
}

// A DMARC aggregate report, in the shape the reporters actually send.
const dmarcReport = `<?xml version="1.0" encoding="UTF-8" ?>
<feedback>
  <report_metadata>
    <org_name>google.com</org_name>
    <report_id>1234567890</report_id>
    <date_range><begin>1000000000</begin><end>1000086400</end></date_range>
  </report_metadata>
  <policy_published>
    <domain>example.com</domain><adkim>r</adkim><aspf>r</aspf>
    <p>none</p><sp>none</sp><pct>100</pct>
  </policy_published>
  <record>
    <row>
      <source_ip>203.0.113.7</source_ip><count>42</count>
      <policy_evaluated><disposition>none</disposition><dkim>pass</dkim><spf>pass</spf></policy_evaluated>
    </row>
    <identifiers><header_from>example.com</header_from></identifiers>
    <auth_results>
      <dkim><domain>example.com</domain><result>pass</result></dkim>
      <spf><domain>example.com</domain><result>pass</result></spf>
    </auth_results>
  </record>
</feedback>`

// A message that is not mail is untouched.
//
// The lookup joins on Ref, which only mail sets. Chat and the web must keep
// going through the escaped path — what somebody typed is text, and rendering
// it as markup would be an injection with extra steps.
func TestAChatMessageIsStillShownAsTyped(t *testing.T) {
	const who = "inbox_render_chat"
	th := thread.Open(who, thread.ChatClient, "xmpp_a_b")
	if th == nil {
		t.Fatal("could not open the conversation")
	}
	t.Cleanup(func() { thread.Forget(who) })
	thread.Add(thread.Message{
		Thread: th.ID, Account: who, Text: "<b>not bold</b>", From: "someone@example.com",
	})

	out := ConversationView(who, th)
	if strings.Contains(out, "<b>not bold</b>") {
		t.Error("what somebody typed was rendered as markup")
	}
	if !strings.Contains(out, "&lt;b&gt;") {
		t.Errorf("the typed text is not shown escaped:\n%s", clip(out))
	}
}

func clip(s string) string {
	if len(s) > 900 {
		return s[:900] + "…"
	}
	return s
}
