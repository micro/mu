package thread

import "testing"

// A search says which part matched.
//
// Because the caller has to show something, and what it should show differs: a
// hit on the words can quote the line, and a hit on the subject or the sender
// cannot — the messages need not contain the query at all. A DMARC report is
// the case that proves it. Its body is "(no message — attached: …)", and
// presenting that as the reason it matched "dmarc" reads as a broken search.
func TestASearchSaysWhichPartMatched(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const who = "where_matched"

	th := Open(who, "mail", "dmarc-chain")
	if th == nil {
		t.Fatal("could not open a conversation")
	}
	Name(who, th.ID, "Report Domain: micro.mu Submitter: google.com")
	Add(Message{Thread: th.ID, Account: who, Text: "(no message — attached: report.zip)"})
	Join(who, th.ID, Party{Kind: RolePerson,
		Key: "noreply-dmarc-support@google.com", Name: "Google"})

	cases := []struct{ query, want string }{
		{"report domain", InSubject},
		{"dmarc", InParty},
		{"google", InSubject}, // the subject names the submitter, and it wins
		{"attached", InText},
	}
	for _, c := range cases {
		hits := Search(who, c.query, "", 20)
		if len(hits) == 0 {
			t.Errorf("%q found nothing", c.query)
			continue
		}
		if hits[0].Where != c.want {
			t.Errorf("%q matched on %q, want %q", c.query, hits[0].Where, c.want)
		}
	}

	// One hit per conversation for a subject match, not one per message —
	// otherwise a long thread whose subject matches buries every other thread
	// that matched once.
	Add(Message{Thread: th.ID, Account: who, Text: "second"})
	Add(Message{Thread: th.ID, Account: who, Text: "third"})
	if hits := Search(who, "report domain", "", 20); len(hits) != 1 {
		t.Errorf("a subject match produced %d hits on one conversation, want 1", len(hits))
	}
}
