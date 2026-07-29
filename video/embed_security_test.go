package video

import (
	"strings"
	"testing"
)

// The video ID arrives straight from ?id= on /video and is interpolated into
// an iframe src attribute.
func TestEmbedRejectsInjectedVideoID(t *testing.T) {
	hostile := []string{
		`abc" onload="alert(1)`,
		`abc"><script>alert(1)</script>`,
		`abc' onerror='alert(1)`,
		`../../evil`,
		`abc?x=1" allow="camera`,
		`javascript:alert(1)`,
	}
	for _, id := range hostile {
		out := embedVideo(id)
		if strings.Contains(out, "onload") || strings.Contains(out, "<script") ||
			strings.Contains(out, "onerror") || strings.Contains(out, "javascript:") {
			t.Errorf("embedVideo(%q) produced markup: %s", id, out)
		}
		if strings.Contains(out, "<iframe") {
			t.Errorf("embedVideo(%q) should not emit an iframe at all: %s", id, out)
		}
	}
}

func TestEmbedAcceptsRealVideoID(t *testing.T) {
	out := embedVideo("dQw4w9WgXcQ")
	if !strings.Contains(out, "https://www.youtube.com/embed/dQw4w9WgXcQ?") {
		t.Errorf("valid ID did not embed: %s", out)
	}
	if !strings.Contains(out, "<iframe") {
		t.Errorf("valid ID should emit an iframe: %s", out)
	}
}
