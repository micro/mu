package home

// A card is drawn twice — once by the page, once by the JSON the page polls
// itself with — and the two have to agree. They did not, and neither render
// could see the other, which is why the difference only showed up as a control
// that vanished a minute after the page loaded.

import (
	"mu/internal/service"
	"strings"
	"testing"
	"time"
)

// The way through to the service survives a refresh.
//
// The page appended the More link to CachedHTML; the JSON sent CachedHTML
// alone, and the script replaces .card-body with it wholesale. So every card
// lost its More link on the first poll and got it back on the next full page
// load — "sometimes the More buttons disappear", from the outside.
func TestACardKeepsItsWayThroughOnRefresh(t *testing.T) {
	c := Card{ID: "news", Title: "News", Link: "/news", CachedHTML: "<p>A headline</p>"}

	body := cardBody(c, service.Anyone())
	if !strings.Contains(body, "<p>A headline</p>") {
		t.Fatalf("the card lost its contents:\n%s", body)
	}
	if !strings.Contains(body, `href="/news"`) {
		t.Errorf("the card has no way through to the service:\n%s", body)
	}
}

// A card with nothing to show is not a card. Both renders have to agree about
// that too, or the page skips one the refresh puts back as an empty box.
func TestAnEmptyCardIsNotDrawn(t *testing.T) {
	if got := cardBody(Card{ID: "news", Title: "News", Link: "/news"}, service.Anyone()); got != "" {
		t.Errorf("an empty card rendered %q", got)
	}
	if got := cardBody(Card{ID: "news", CachedHTML: "   \n "}, service.Anyone()); got != "" {
		t.Errorf("a card of whitespace rendered %q", got)
	}
}

// The title is the name of the service, so it goes there.
func TestACardTitleLinksToItsService(t *testing.T) {
	head := cardHead(Card{ID: "news", Title: "News", Link: "/news"})
	if !strings.Contains(head, `href="/news"`) {
		t.Errorf("the title does not link to the service:\n%s", head)
	}
	if !strings.Contains(head, ">News<") {
		t.Errorf("the title lost its name:\n%s", head)
	}

	// A card with nowhere to go is still a card, and its title is text.
	plain := cardHead(Card{ID: "nowhere", Title: "Nowhere"})
	if strings.Contains(plain, "<a") {
		t.Errorf("a card with no link still drew one:\n%s", plain)
	}
}

// The age moves when the contents do. It is built into the title, the title was
// sent in the JSON and never used by the script, so a card refreshed its
// headlines and went on claiming they were an hour old.
func TestACardSaysHowOldItIs(t *testing.T) {
	head := cardHead(Card{
		ID: "news", Title: "News", Link: "/news",
		At: time.Now().Add(-2 * time.Hour),
	})
	if !strings.Contains(head, "card-when") {
		t.Errorf("a card that happened at a time does not say when:\n%s", head)
	}

	// A standing view has no age — markets is how things are, not something
	// that occurred — so it must not grow a timestamp that never changes.
	standing := cardHead(Card{ID: "markets", Title: "Markets", Link: "/markets"})
	if strings.Contains(standing, "card-when") {
		t.Errorf("a standing card claimed a time:\n%s", standing)
	}
}
