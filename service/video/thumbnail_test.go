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

// videos.json stores rendered markup, so a stale cache keeps serving Google's
// URLs after the code changed. This is what made the first fix look deployed
// and have no effect on the page.
func TestProxyThumbnailsHealsCachedMarkup(t *testing.T) {
	cached := `<div class="thumbnail"><a href="/video?id=hE2HEj1JBcI">` +
		`<img src="https://i.ytimg.com/vi/hE2HEj1JBcI/mqdefault.jpg"><h3>A video</h3></a></div>` +
		`<img src="http://i.ytimg.com/vi/_ssYCLUfv9k/hqdefault.jpg">`

	got := ProxyThumbnails(cached)
	if strings.Contains(got, "ytimg.com") {
		t.Errorf("a Google URL survived the rewrite:\n%s", got)
	}
	for _, want := range []string{"/video/thumb?id=hE2HEj1JBcI", "/video/thumb?id=_ssYCLUfv9k"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Everything else is left alone.
	if !strings.Contains(got, `<h3>A video</h3>`) || !strings.Contains(got, `href="/video?id=hE2HEj1JBcI"`) {
		t.Errorf("the rewrite damaged surrounding markup:\n%s", got)
	}
}

// It must not touch anything that only looks like a thumbnail URL.
func TestProxyThumbnailsLeavesOtherURLsAlone(t *testing.T) {
	for _, in := range []string{
		`<a href="https://www.youtube.com/watch?v=hE2HEj1JBcI">x</a>`,
		`<img src="https://evil.example.com/i.ytimg.com/vi/hE2HEj1JBcI/mq.jpg">`,
		`<img src="/images/local.jpg">`,
	} {
		if got := ProxyThumbnails(in); got != in {
			t.Errorf("rewrote %q into %q", in, got)
		}
	}
}
