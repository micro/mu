package video

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChannelLinksStayLocal(t *testing.T) {
	got := channelLink("A & B", "UC123")
	if !strings.Contains(got, `href="/video?channel=UC123"`) || !strings.Contains(got, "A &amp; B") {
		t.Fatal(got)
	}
	if got := channelLink("Plain", ""); got != "Plain" {
		t.Fatal(got)
	}
}

func TestWatchHasAudioControls(t *testing.T) {
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest("GET", "https://micro.mu/video?id=abcdefghijk&audio=1", nil))
	for _, want := range []string{`id="audioBtn"`, `id="playBtn"`, `id="ytplayer"`, `enablejsapi=1`, `/mu.js?v=video-1`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("missing %s", want)
		}
	}
}
