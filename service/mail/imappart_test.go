package mail

// Fetching one part of a message by number, which is how a mail client gets an
// attachment.

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// A message with an attachment hands over the attachment.
//
// imapBodyStructure has always described these correctly — a two-part MIXED —
// so a client does what it is told, asks for BODY[2], and until this was
// written got an empty literal back. What it showed instead was the line the
// body carries in place of the attachment, "[report.zip (application/zip), 684
// bytes — not shown]", with no file behind it, while the same message rendered
// its report on the web. A DMARC report is the case: it is the attachment this
// instance receives constantly.
func TestAnAttachmentIsFetchableByPartNumber(t *testing.T) {
	zip := []byte("PK\x03\x04 pretend this is a zip")
	m := &Message{
		ID: "m1", From: "noreply@google.com", To: "asim", ToID: "asim",
		Subject: "Report domain: micro.mu", CreatedAt: time.Now(),
		Body:           "[google.com!micro.mu!1788307200!1788393599.zip (application/zip), 684 bytes — not shown]",
		Attachment:     base64.StdEncoding.EncodeToString(zip),
		AttachmentName: "google.com!micro.mu.zip",
		AttachmentType: "application/zip",
	}
	full := imapRender(m)

	// Part 1 is the text, and only the text: no boundary, no base64.
	one := string(imapPart(full, 1))
	if !strings.Contains(one, "not shown]") {
		t.Errorf("part 1 is not the message text: %q", one)
	}
	if strings.Contains(one, "--mu-") || strings.Contains(one, m.Attachment) {
		t.Errorf("part 1 carries the rest of the message with it: %q", one)
	}

	// Part 2 is the attachment, as transmitted — base64, which is what
	// BODYSTRUCTURE tells the client to expect.
	two := strings.ReplaceAll(string(imapPart(full, 2)), "\r\n", "")
	if two == "" {
		t.Fatal("part 2 is empty, so the attachment cannot be saved by any client")
	}
	got, err := base64.StdEncoding.DecodeString(two)
	if err != nil {
		t.Fatalf("part 2 is not the base64 the structure promised: %v", err)
	}
	if string(got) != string(zip) {
		t.Errorf("part 2 decodes to %q, want the file that arrived", got)
	}

	// And a part that does not exist is empty rather than something else's.
	if p := imapPart(full, 3); len(p) != 0 {
		t.Errorf("part 3 of a two-part message returned %q", p)
	}

	// Through the command a client actually sends, because the parts being
	// right is no use if BODY[2] does not reach them — which is exactly how
	// this shipped: imapPart's job was being done by the branch for BODY[TEXT].
	sess := &imapSession{account: "asim", folder: "INBOX", readOnly: true,
		msgs: []*Message{m}, uids: []uint32{1}}
	reply := sess.fetchBody(m, "BODY.PEEK[2]")
	if !strings.Contains(reply, "BODY[2] {") {
		t.Fatalf("BODY.PEEK[2] answered %q", clip(reply, 200))
	}
	payload := strings.ReplaceAll(reply[strings.Index(reply, "}\r\n")+3:], "\r\n", "")
	if payload == "" {
		t.Fatal("BODY[2] came back empty, which is the bug this fixes")
	}
	if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
		t.Errorf("BODY[2] is not the attachment: %q", clip(payload, 200))
	}
	// And BODY[1] over the same path is the text alone.
	if got := sess.fetchBody(m, "BODY.PEEK[1]"); strings.Contains(got, "--mu-") {
		t.Errorf("BODY[1] carries the whole multipart body: %q", clip(got, 200))
	}
}


// A message of one part is one part: the body, and nothing at 2.
func TestASimpleMessageHasOnePart(t *testing.T) {
	m := &Message{ID: "m2", From: "a@example.test", To: "asim", ToID: "asim",
		Subject: "Hello", Body: "Just text.", CreatedAt: time.Now()}
	full := imapRender(m)

	if got := strings.TrimSpace(string(imapPart(full, 1))); got != "Just text." {
		t.Errorf("part 1 = %q, want the body", got)
	}
	if p := imapPart(full, 2); len(p) != 0 {
		t.Errorf("a message with no attachment has a part 2: %q", p)
	}
}

// The boundary is read from the message rather than assumed, because it is
// built from the message id and two messages do not share one.
func TestTheBoundaryComesFromTheMessage(t *testing.T) {
	m := &Message{ID: "abc", Body: "x", Attachment: base64.StdEncoding.EncodeToString([]byte("y")),
		AttachmentName: "y.bin", AttachmentType: "application/octet-stream", CreatedAt: time.Now()}
	head, _ := imapSplitMessage(imapRender(m))
	if got := imapBoundary(head); got != "mu-abc" {
		t.Errorf("boundary = %q, want the one the message declares", got)
	}
	if got := imapBoundary([]byte("Content-Type: text/plain\r\n\r\n")); got != "" {
		t.Errorf("a single-part message declared a boundary: %q", got)
	}
}
