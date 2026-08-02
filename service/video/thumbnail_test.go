package video

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The proxy builds its own upstream URL from an id it has validated, so there
// is no caller-supplied URL that could point at something else.
func TestThumbnailRejectsAnythingButAVideoID(t *testing.T) {
	for _, bad := range []string{
		"", "short", "waytoolongforanid", "../../etc/passwd",
		"http://169.254.169.254/", "abc/../../x", "abcdefghij!", "abcdefghij k",
	} {
		if ThumbURL(bad) != "" {
			t.Errorf("ThumbURL(%q) produced a URL", bad)
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/video/thumb", nil)
		q := req.URL.Query()
		q.Set("id", bad)
		req.URL.RawQuery = q.Encode()
		ThumbHandler(rec, req)
		if rec.Code != 400 {
			t.Errorf("id %q returned %d, want 400", bad, rec.Code)
		}
	}
}

func TestThumbnailAcceptsARealID(t *testing.T) {
	if got := ThumbURL("hE2HEj1JBcI"); got != "/video/thumb?id=hE2HEj1JBcI" {
		t.Errorf("ThumbURL = %q", got)
	}
}

// A page of videos must not reach out to Google's CDN from the reader's
// browser — that is the tracking /video exists to avoid, and it is what breaks
// when a content blocker takes out i.ytimg.com.
func TestFeedDoesNotLinkToGooglesCDN(t *testing.T) {
	html := Latest()
	if html == "" {
		t.Skip("no cached feed in this binary")
	}
	if strings.Contains(html, "ytimg.com") || strings.Contains(html, "img.youtube.com") {
		t.Error("feed still links thumbnails straight to Google's CDN")
	}
}
