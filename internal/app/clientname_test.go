package app

import "testing"

// Every client the record writes has a name for a reader.
//
// The default — show the stored value as it names itself — is right for
// something nobody has got to yet and wrong for a door this product ships: a
// conversation over XMPP sat in the inbox labelled a lowercase "chat" beside a
// capitalised "Mail", which reads as a bug in the page rather than a client
// nobody has named.
func TestEveryClientTheRecordWritesHasAName(t *testing.T) {
	for client, want := range map[string]string{
		"web":  "Web",
		"mail": "Mail",
		"chat": "Chat",
		"cli":  "CLI",
	} {
		if got := ClientName(client); got != want {
			t.Errorf("ClientName(%q) = %q, want %q", client, got, want)
		}
	}
}

// One thing is called one thing.
//
// "Email" and "Mail" for the same channel is a second name, and the page it
// shows on is the one page where it sits beside other channels — so the two
// words read as two of them.
func TestMailIsNotAlsoCalledEmail(t *testing.T) {
	if got := ClientName("mail"); got == "Email" {
		t.Error("mail is labelled Email, which is a second name for the service, " +
			"the route, the address and the docs, all of which say mail")
	}
}

// A client nobody has named still says something.
func TestAnUnknownClientShowsAsItNamesItself(t *testing.T) {
	if got := ClientName("carrier-pigeon"); got != "carrier-pigeon" {
		t.Errorf("ClientName of an unknown client = %q, want it echoed back", got)
	}
}
