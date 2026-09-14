package app

// The chat sizes itself against what is on screen, not what the window says.

import (
	"strings"
	"testing"
)

// window.innerHeight is the layout viewport, and on iOS a keyboard does not
// change it — the keyboard overlays the visual viewport instead. So fitConv
// sized the conversation to the full height of a window whose bottom third was
// covered, and the composer underneath ended up beneath the keyboard: on a
// phone, in a standalone app, that is the box you were about to type in.
//
// And it never re-ran, because iOS fires no window resize for a keyboard —
// nothing about the layout viewport changed. visualViewport is where both
// facts live.
func TestTheChatFitsWhatIsActuallyOnScreen(t *testing.T) {
	js := ChatComponent(ChatConfig{Ask: true})

	if !strings.Contains(js, "v?v.height:window.innerHeight") {
		t.Error("the conversation is sized from window.innerHeight alone, which\n" +
			"does not change when an iOS keyboard opens — so it fills the\n" +
			"screen and the composer goes under the keyboard")
	}
	if !strings.Contains(js, `window.visualViewport.addEventListener('resize'`) {
		t.Error("nothing listens for the visual viewport changing, so opening a\n" +
			"keyboard never re-runs the fit — iOS fires no window resize for it")
	}
	// scroll as well: the visual viewport also moves when the page is panned
	// with a keyboard up, and the height sized against goes stale with it.
	if !strings.Contains(js, `window.visualViewport.addEventListener('scroll'`) {
		t.Error("the fit does not follow the visual viewport when it moves")
	}
	// And it still works where there is no visualViewport, which is what every
	// desktop browser wants anyway.
	if !strings.Contains(js, ":window.innerHeight") {
		t.Error("there is no fallback for a browser with no visualViewport")
	}

}

// A browser or proxy may drop the SSE connection while the independently
// running agent continues. The page must follow the recorded conversation in
// that case rather than turning a transport failure into the run's outcome.
func TestAChatRecoversTheAnswerAfterItsStreamDrops(t *testing.T) {
	js := ChatComponent(ChatConfig{Ask: true, StorageNS: "probe"})

	if !strings.Contains(js, "Reconnecting...") {
		t.Error("a dropped stream is still presented as a failed run")
	}
	if strings.Count(js, "'/agent/pending?thread='") < 2 {
		t.Error("the live request does not poll the recorded conversation after its stream drops")
	}
}
