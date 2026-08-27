package mail

import (
	"strings"
	"testing"

	"mu/internal/data"
)

// indexed pins the real index on for a test.
//
// It is the default now, and this stays because the default is a default: an
// instance can set MU_USE_SQLITE=0, and a test that quietly followed it would
// exercise the in-memory fallback instead — a map scanned with strings.Contains
// and populated through a queue by background workers. Neither half is what
// these tests are about, and the queue makes them racy.
func indexed(t *testing.T) {
	t.Helper()
	was := data.UseSQLite
	data.UseSQLite = true
	t.Cleanup(func() { data.UseSQLite = was })
}

// A search finds mail, and only the searcher's.
//
// The scan this replaced iterated every message on the instance and filtered by
// owner inside the loop. The filter was correct; putting it in a loop was the
// problem, and the index does the scoping itself — which is also what makes
// getting it wrong loud rather than quiet.
func TestSearchFindsYourMailAndNobodyElses(t *testing.T) {
	indexed(t)
	t.Setenv("MAIL_DOMAIN", "example.test")
	mine := account(t, "searcher")
	theirs := account(t, "stranger")

	if err := SendMessage("acme@example.com", "acme@example.com", mine, mine,
		"Invoice 4021 from Acme", "The total is 420 pounds, due Friday.", "", "<inv-1@acme>"); err != nil {
		t.Fatal(err)
	}
	if err := SendMessage("acme@example.com", "acme@example.com", theirs, theirs,
		"Invoice 9999 from Acme", "A stranger's invoice, also from Acme.", "", "<inv-2@acme>"); err != nil {
		t.Fatal(err)
	}

	hits := Search(mine, "Invoice 4021", 10)
	if len(hits) == 0 {
		t.Fatal("searching for a message that exists found nothing")
	}
	if !strings.Contains(hits[0].Subject, "4021") {
		t.Errorf("the best hit is %q, want the invoice searched for", hits[0].Subject)
	}
	for _, h := range hits {
		if h.ToID == theirs {
			t.Error("a search returned somebody else's mail")
		}
	}

	// The sender is searchable too, because "everything from Acme" is a search
	// people do and the address is not in the subject or the body.
	if len(Search(mine, "acme", 10)) == 0 {
		t.Error("searching by sender found nothing")
	}
}

// Deleting a message stops it being found.
//
// The index had no opposite until now: Index and IndexOwned existed and nothing
// removed. An index that cannot forget is not a cache, it is a second copy that
// outlives the first — deleted mail stays findable, and the deletion hooks that
// clear every store leave this one holding what they cleared.
func TestDeletedMailIsNotFound(t *testing.T) {
	indexed(t)
	t.Setenv("MAIL_DOMAIN", "example.test")
	who := account(t, "deleter")

	if err := SendMessage("someone@example.com", "someone@example.com", who, who,
		"Weekend plans", "Pub at seven, bring the dog.", "", "<del-1@x>"); err != nil {
		t.Fatal(err)
	}
	hits := Search(who, "Weekend plans", 10)
	if len(hits) == 0 {
		t.Fatal("the message was not findable before deleting it, so this test " +
			"cannot tell deletion from never having worked")
	}

	if err := DeleteMessage(hits[0].ID, who); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := Search(who, "Weekend plans", 10); len(got) != 0 {
		t.Errorf("%d hits after deleting the only message — the mail is gone and "+
			"a search still finds it", len(got))
	}
}

// Deleting an account takes its mail out of the index too.
func TestDeletingAnAccountEmptiesItsSearch(t *testing.T) {
	indexed(t)
	t.Setenv("MAIL_DOMAIN", "example.test")
	who := account(t, "departing")

	if err := SendMessage("someone@example.com", "someone@example.com", who, who,
		"Something memorable", "A distinctive phrase nobody else would write.", "", "<dep-1@x>"); err != nil {
		t.Fatal(err)
	}
	if len(Search(who, "distinctive phrase", 10)) == 0 {
		t.Fatal("not findable to begin with")
	}

	DeleteInbox(who)
	if got := Search(who, "distinctive phrase", 10); len(got) != 0 {
		t.Errorf("%d hits after the account's mail was deleted", len(got))
	}
}
