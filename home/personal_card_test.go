package home

// A personal card answers the reader, and never enters the shared cache.
//
// Both halves matter and they are the same rule seen from two sides. Home keeps
// one set of card strings for every viewer, so a personal render in that cache
// would be served to whoever asked next — and enforcing that by rendering every
// card impersonally, which is what happened before, meant no card could ever
// answer anybody. Four services had written the personal branch and none of it
// was reachable here.

import (
	"strings"
	"testing"
	"time"

	"mu/internal/auth"
	"mu/internal/service"
)

// withCards swaps the package's cards for the duration of a test.
func withCards(t *testing.T, cards []Card) {
	t.Helper()
	before := Cards
	Cards = cards
	t.Cleanup(func() { Cards = before })
}

// personal is a card that says who it was drawn for.
func personal(id string) Card {
	return Card{
		ID:    id,
		Title: id,
		Content: service.Personal(func(v service.Viewer) string {
			if v.Account == "" {
				return "<p>nobody in particular</p>"
			}
			return "<p>for " + v.Account + "</p>"
		}),
	}
}

// The refresh renders and caches the shared cards, and leaves personal ones
// alone — including not rendering them, which is a fetch saved: drawing weather
// as Anyone every two minutes is a forecast fetched for nobody.
func TestTheCacheSkipsAPersonalCard(t *testing.T) {
	shared := Card{
		ID:      "news",
		Title:   "News",
		Content: service.Glance(func() string { return "<p>a headline</p>" }),
	}
	withCards(t, []Card{shared, personal("weather")})

	ForceRefresh()

	if got := Cards[0].CachedHTML; !strings.Contains(got, "a headline") {
		t.Errorf("the shared card was not cached: %q", got)
	}
	if got := Cards[1].CachedHTML; got != "" {
		t.Errorf("a personal card reached the shared cache: %q", got)
	}
}

// And the page draws it for whoever is looking.
func TestAPersonalCardIsDrawnForTheReader(t *testing.T) {
	withCards(t, []Card{personal("weather")})
	ForceRefresh()

	mine := cardBody(Cards[0], service.For("asim"))
	if !strings.Contains(mine, "for asim") {
		t.Errorf("the card did not answer the reader: %q", mine)
	}

	theirs := cardBody(Cards[0], service.For("someone-else"))
	if strings.Contains(theirs, "asim") {
		t.Errorf("one reader's card was served to another: %q", theirs)
	}

	out := cardBody(Cards[0], service.Anyone())
	if !strings.Contains(out, "nobody in particular") {
		t.Errorf("the signed-out render is not the impersonal one: %q", out)
	}
}

// A shared card is still cached, so it is not re-rendered per reader.
func TestASharedCardStillComesFromTheCache(t *testing.T) {
	var renders int
	withCards(t, []Card{{
		ID:      "news",
		Title:   "News",
		Content: service.Glance(func() string { renders++; return "<p>a headline</p>" }),
	}})
	ForceRefresh()

	before := renders
	for i := 0; i < 5; i++ {
		if got := cardBody(Cards[0], service.For("asim")); !strings.Contains(got, "a headline") {
			t.Fatalf("the cached card did not render: %q", got)
		}
	}
	if renders != before {
		t.Errorf("a shared card re-rendered %d times per read; it should come "+
			"from the cache", renders-before)
	}
}

// What the agent is told is what the reader is looking at, personal cards
// included. This is the same bug in the other direction: the model was handed
// the signed-out branch of a card the reader was not seeing.
func TestTheAgentContextGetsThePersonalRender(t *testing.T) {
	withCards(t, []Card{personal("weather")})
	ForceRefresh()

	ctx := CardContext(&auth.Account{ID: "asim", Created: time.Now()})
	if !strings.Contains(ctx, "for asim") {
		t.Errorf("the agent was not told what the reader can see: %q", ctx)
	}
	if strings.Contains(ctx, "nobody in particular") {
		t.Errorf("the agent got the signed-out render: %q", ctx)
	}
}

// The distinction has to survive the wrappers, since that is the whole change:
// Glance and Personal already knew, and the shared function type forgot.
func TestTheWrappersSayWhichKindTheyAre(t *testing.T) {
	if service.Personal(func(service.Viewer) string { return "" }).Personal() != true {
		t.Error("Personal does not report itself as personal")
	}
	if service.Glance(func() string { return "" }).Personal() {
		t.Error("Glance reports itself as personal")
	}
	if service.Timed(func() (string, time.Time) { return "", time.Time{} }).Personal() {
		t.Error("Timed reports itself as personal")
	}
	var none service.Renderer
	if none.Set() {
		t.Error("the zero renderer claims to render something")
	}
	if got := none.Render(service.Anyone()); got.HTML != "" {
		t.Errorf("the zero renderer drew %q instead of nothing", got.HTML)
	}
}
