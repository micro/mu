package auth

import "testing"

func TestShowHomeCard(t *testing.T) {
	cases := []struct {
		name string
		acc  Account
		id   string
		want bool
	}{
		{"fresh account shows every default", Account{}, "images", true},
		{"fresh account shows prayer under its oldest id", Account{}, "reminder", true},
		{"fresh account shows prayer under its middle id", Account{}, "islam", true},
		{"legacy set (no seen) shows new card images",
			Account{HomeCards: []string{"blog", "news", "markets", "reminder", "social", "video"}}, "images", true},
		{"legacy set keeps a chosen card",
			Account{HomeCards: []string{"blog", "news"}}, "blog", true},
		{"legacy set hides a pre-existing deselected card",
			Account{HomeCards: []string{"blog", "news"}}, "social", false},
		{"post-save deselect of images sticks",
			Account{HomeCards: []string{"blog", "news"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "images", false},
		{"post-save select of images shows",
			Account{HomeCards: []string{"blog", "images"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "images", true},
		{"future card after a save defaults on",
			Account{HomeCards: []string{"blog"}, HomeCardsSeen: []string{"blog", "news", "markets", "reminder", "social", "video", "images", "mail", "web"}}, "audio", true},
	}
	for _, c := range cases {
		if got := c.acc.ShowHomeCard(c.id); got != c.want {
			t.Errorf("%s: ShowHomeCard(%q)=%v want %v", c.name, c.id, got, c.want)
		}
	}
}

func TestHomeCardActiveOptIn(t *testing.T) {
	if (&Account{}).HomeCardActive("mail") {
		t.Error("fresh account should not have mail opt-in active")
	}
	if !(&Account{HomeCards: []string{"mail"}}).HomeCardActive("mail") {
		t.Error("mail in HomeCards should be active")
	}
}

// A card id that was renamed must keep resolving from stored preferences. This
// card has been renamed twice — "reminder", then "islam", now "prayer" — so
// both old ids have to canonicalise, and a deliberate hide still sticks.
func TestLegacyCardIDStillResolves(t *testing.T) {
	for _, old := range []string{"reminder", "islam"} {
		seenOld := []string{"blog", "news", "markets", old, "social", "video", "images", "mail", "web"}

		selected := Account{HomeCards: []string{"blog", old}, HomeCardsSeen: seenOld}
		if !selected.ShowHomeCard("prayer") {
			t.Errorf("a stored %q selection should show the prayer card", old)
		}

		hidden := Account{HomeCards: []string{"blog"}, HomeCardsSeen: seenOld}
		if hidden.ShowHomeCard("prayer") {
			t.Errorf("prayer was offered as %q and not selected — should stay hidden", old)
		}
	}

	// Legacy account with no seen-set: prayer is not a new card, so a
	// customised account that dropped it must not have it reappear.
	legacy := Account{HomeCards: []string{"blog", "news"}}
	if legacy.ShowHomeCard("prayer") {
		t.Error("prayer predates seen-tracking; should not resurface for a customised legacy account")
	}
}
