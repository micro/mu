package mail

// An agent answers in markdown, and mail has to render it.
//
// Every other surface already did: the web page renders, Discord and Telegram
// normalise, and the inbox itself runs markdown through app.RenderString when it
// arrives the other way. Mail was the one place an answer went out raw, so a
// reply with a list or a bold word arrived with its asterisks showing.
//
// The reply is built inside answerMail, so this reads the source. It
// is checking a wiring decision — which arguments the send is given — and that
// is exactly what a source read can see.

import (
	"os"
	"strings"
	"testing"
)

func replySource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("mail.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// SendExternalReplyAll, since the agent got copied into other people's
	// threads and a reply to the sender alone leaves the rest of the room
	// watching half a conversation. What these tests hold is unchanged: what
	// goes out is rendered, has a plain alternative, and is not trusted.
	i := strings.Index(s, "mail.SendExternalReplyAll(")
	if i < 0 {
		t.Fatal("the agent no longer replies to mail through SendExternalReplyAll")
	}
	end := strings.Index(s[i:], ")")
	if end < 0 {
		t.Fatal("cannot bound the send call")
	}
	return s[i : i+end]
}

func TestTheAgentsMailReplyIsRendered(t *testing.T) {
	call := replySource(t)

	if strings.Contains(call, `, "",`) {
		t.Error("the reply is sent with an empty HTML body, so the answer goes out as " +
			"raw markdown and arrives with its asterisks showing")
	}
	if !strings.Contains(call, "RenderString") {
		t.Error("the reply does not render its markdown")
	}
	if !strings.Contains(call, "plain") {
		t.Error("the reply no longer sends a plain-text alternative — a client that " +
			"prefers text would get nothing")
	}
}

// Rendered untrusted, because the body is model output.
//
// RenderTrusted passes raw HTML through and exists for content that ships in the
// binary. An answer is not that: it is a string a model wrote, downstream of
// text a stranger emailed in.
func TestTheAgentsMailReplyIsNotRenderedAsTrusted(t *testing.T) {
	// The call itself, not the file: the comment above it names RenderTrusted
	// to say why it is not used, and a file-wide search matches the explanation
	// rather than the code.
	if call := replySource(t); strings.Contains(call, "RenderTrusted") {
		t.Error("the mail reply renders as trusted — a body is model output and raw " +
			"HTML in it must be escaped, not passed through")
	}
}
