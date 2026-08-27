package inbox

// Writing to a person by name.
//
// The Write button on a profile puts @someone in the To box rather than their
// address, because /@somebody is a public page and the address was printed on
// it. The shortcut is the point and the address is not the sender's business,
// so the resolution happens here, on the way out.

import (
	"strings"
	"testing"

	"mu/internal/auth"
	"mu/internal/thread"
)

func TestAnAtNameResolvesToTheirAddress(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if err := auth.Create(&auth.Account{ID: "henrik", Name: "Henrik", Secret: "s"}); err != nil {
		t.Fatalf("creating the account: %v", err)
	}

	got, ok := addressOfPerson("@henrik")
	if !ok {
		t.Fatal("@henrik resolved to nobody, so Write from their profile sends nothing")
	}
	if got != "henrik@mu.test" {
		t.Errorf("addressOfPerson(@henrik) = %q", got)
	}

	// Their address, not their agent's — a tag here would quietly run
	// somebody's agent instead of writing to them. service/mail draws that
	// line: untagged mail is just mail.
	if want := "henrik@mu.test"; got != want {
		t.Errorf("the resolved address carries a tag: %q", got)
	}

	// However it is written down.
	if got, ok := addressOfPerson("@HENRIK"); !ok || got != "henrik@mu.test" {
		t.Errorf("@HENRIK resolved to %q/%v — an account id is one name", got, ok)
	}

	// And a name nobody answers to is refused rather than turned into an
	// address that would bounce.
	if _, ok := addressOfPerson("@nobody-here"); ok {
		t.Error("a name with no account behind it produced an address")
	}
	if _, ok := addressOfPerson("@"); ok {
		t.Error("a bare @ produced an address")
	}
}

// /@somebody is the conversation with them, and finding it is by who is on it.
//
// Every reader over the record was keyed by conversation — an id, a client and
// key, an account. None of them could answer "what have this person and I said
// to each other", which is the question the page is. Parties was recorded and
// read by nothing but search.
func TestTheirPageFindsTheConversationsWithThem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	// Tolerant of accounts another test in this package already made: auth
	// keeps them in memory for the process, so HOME being a fresh directory
	// does not make the names fresh too.
	for _, id := range []string{"mine", "henrik", "stranger"} {
		if _, err := auth.GetAccount(id); err == nil {
			continue
		}
		if err := auth.Create(&auth.Account{ID: id, Name: strings.ToTitle(id), Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}

	mine := thread.Open("mine", "mail", "henrik@mu.test")
	thread.Join("mine", mine.ID, thread.Party{Kind: thread.RolePerson, Key: "henrik@mu.test"})
	theirs := thread.Open("mine", "mail", "stranger@mu.test")
	thread.Join("mine", theirs.ID, thread.Party{Kind: thread.RolePerson, Key: "stranger@mu.test"})

	got := thread.With("mine", namesFor("henrik")...)
	if len(got) != 1 {
		t.Fatalf("Henrik's page shows %d conversations, want 1", len(got))
	}
	if got[0].ID != mine.ID {
		t.Error("Henrik's page is showing somebody else's conversation")
	}

	// And an account nobody has written to has none, which is what makes the
	// blank message the empty state rather than a special case.
	if n := len(thread.With("mine", namesFor("nobody")...)); n != 0 {
		t.Errorf("an account with no history has %d conversations", n)
	}
}

// A name that is a prefix of another does not collect their conversations.
//
// Search matches substrings and can afford to: a near-miss costs a reader a
// glance. This decides whose page a conversation is on, so "sam" matching
// samantha@ would put one person's correspondence in front of another.
func TestOnePersonsPageIsNotAnothersHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if _, err := auth.GetAccount("owner"); err != nil {
		if err := auth.Create(&auth.Account{ID: "owner", Name: "Owner", Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	long := thread.Open("owner", "mail", "samantha@mu.test")
	thread.Join("owner", long.ID, thread.Party{Kind: thread.RolePerson, Key: "samantha@mu.test"})

	if got := thread.With("owner", "sam"); len(got) != 0 {
		t.Errorf("a partial name collected %d of somebody else's conversations", len(got))
	}
	if got := thread.With("owner", "samantha@mu.test"); len(got) != 1 {
		t.Errorf("the whole address found %d conversations, want 1", len(got))
	}
}

// The reply on their page goes to them, whoever spoke last.
//
// replyTo works it out from the messages, which is right in the inbox and
// wrong here: a conversation you started has nobody but you in it, so it
// produced no Reply at all and a note telling you to answer it "the way it
// arrived" — on the page it arrived on.
func TestTheReplyOnTheirPageGoesToThem(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MAIL_DOMAIN", "mu.test")

	if _, err := auth.GetAccount("henrik2"); err != nil {
		if err := auth.Create(&auth.Account{ID: "henrik2", Name: "Henrik", Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	mail := &thread.Thread{ID: "t1", Client: "mail", Key: "henrik2@mu.test"}
	if got := replyAddressFor("henrik2", mail); got != "henrik2@mu.test" {
		t.Errorf("a reply on Henrik's page goes to %q", got)
	}

	// And a text is answered with a text: the transport belongs to the
	// conversation, not to the person.
	text := &thread.Thread{ID: "t2", Client: "sms", Key: "+447700900123"}
	if got := replyAddressFor("henrik2", text); got != "+447700900123" {
		t.Errorf("a reply to a text goes to %q rather than the number", got)
	}
}
