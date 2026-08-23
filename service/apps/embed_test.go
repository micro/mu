package apps

// Embedding: the tag, and what it refuses.
//
// The tag itself is four attributes and hard to get wrong. What is worth
// holding is the two refusals — a paid app, whose embedded copy would not
// charge, and somebody else's private app, which GetApp will hand over to
// anyone who guesses the slug.

import (
	"strings"
	"testing"
)

func TestTheEmbedTagPointsAtTheAppItself(t *testing.T) {
	got := EmbedHTML("https://micro.mu/", "pomodoro-timer", "Pomodoro Timer")

	// The raw document, not our page around it. /apps/<slug> carries the
	// chrome, the title and the bridge, and framing that gives somebody else a
	// window onto our furniture.
	if !strings.Contains(got, `src="https://micro.mu/apps/pomodoro-timer?raw=1"`) {
		t.Errorf("the tag does not point at the app document: %s", got)
	}
	if !strings.HasPrefix(got, "<iframe ") {
		t.Errorf("the tag is not an iframe: %s", got)
	}
	// The frame's own sandbox, on top of the response's. A site pasting this
	// should not have to trust that we set a header.
	if !strings.Contains(got, `sandbox="allow-scripts`) {
		t.Errorf("the tag does not sandbox the frame: %s", got)
	}
	if strings.Contains(got, "allow-same-origin") {
		t.Errorf("the tag lets the app out of its origin: %s", got)
	}
}

// An app that talks to Mu is flagged, because off-site nothing answers it: the
// shim posts to a parent with no bridge, waits sixty seconds and fails.
func TestAnAppThatNeedsTheBridgeIsRecognised(t *testing.T) {
	for _, uses := range []string{
		`<script>mu.store.set('k', 1)</script>`,
		`<script>window.mu.news().then(render)</script>`,
		`<script>const r = await mu.db.list('notes')</script>`,
	} {
		if !bridged(uses) {
			t.Errorf("a bridge call was not spotted: %s", uses)
		}
	}
	for _, plain := range []string{
		`<script>let mus = 1; document.title = 'timer'</script>`,
		`<p>A stopwatch. No network.</p>`,
	} {
		if bridged(plain) {
			t.Errorf("a plain app was flagged as needing the bridge: %s", plain)
		}
	}
}
