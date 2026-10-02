package mail

import (
	"encoding/json"
	"mu/internal/auth"
	"strings"
	"testing"
)

func TestIMAPGeneratedMailKeepsFormatAndThreadHeaders(t *testing.T) {
	original := Message{ID: "internal-child", ReplyTo: "internal-parent", MessageID: "<child@example.test>", InReplyTo: "<parent@example.test>", References: "<root@example.test>", Body: "## Daily Checkin\n\nHow is your day?", Markdown: true}
	saved, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var m Message
	if err := json.Unmarshal(saved, &m); err != nil {
		t.Fatal(err)
	}
	wire := string(imapRender(&m))
	for _, want := range []string{"Content-Type: text/html", "In-Reply-To: <parent@example.test>", "References: <root@example.test> <parent@example.test>", "Daily Checkin</h2>"} {
		if !strings.Contains(wire, want) {
			t.Fatalf("missing %q in %s", want, wire)
		}
	}
	if strings.Contains(wire, "internal-parent") || strings.Contains(wire, "## Daily") {
		t.Fatal("internal ID or raw markdown escaped into email")
	}
	if !strings.Contains(imapEnvelope(&m), "<parent@example.test>") {
		t.Fatal("envelope lost parent")
	}
}

func TestLocalSubmissionKeepsClientMessageID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	owner := "submission-identity"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	clientID := "<gmail-original@example.test>"
	session := &submissionSession{acc: &auth.Account{ID: owner}}
	if err := session.deliverLocally("agent@"+ConfiguredDomain(), owner, "Re: Daily Checkin", "My focus", "", "<checkin@example.test>", "<checkin@example.test>", clientID); err != nil {
		t.Fatal(err)
	}
	stored := FindMessageByMessageID(clientID)
	if stored == nil || stored.FromID != EmailForUser(owner, ConfiguredDomain()) || stored.InReplyTo != "<checkin@example.test>" {
		t.Fatalf("client identity lost: %+v", stored)
	}
}

func TestMailReplyUsesReservedIdentity(t *testing.T) {
	const ref = "<reserved-reply@example.test>"
	body, id := buildExternalTo("Micro", "agent@example.test", "", "person@outside.test", nil, "Re: Checkin", "Answer", "<p>Answer</p>", "<question@example.test>", "", ref)
	if id != ref || !strings.Contains(string(body), "Message-ID: "+ref+"\r\n") {
		t.Fatal("outbound reply replaced reserved identity")
	}
	const owner = "reserved-reply-owner"
	auth.SetAccountForTest(&auth.Account{ID: owner, Approved: true})
	defer auth.RemoveAccountForTest(owner)
	got, err := SendReplyAll(owner, "Micro", "agent@"+ConfiguredDomain(), owner, nil, "Re: Checkin", "Answer", "<p>Answer</p>", "<question@example.test>", "", ref)
	if err != nil {
		t.Fatal(err)
	}
	stored := FindMessageByMessageID(ref)
	if got != ref || stored == nil || stored.ToID != owner {
		t.Fatalf("local reply replaced reserved identity: %q %+v", got, stored)
	}
}
