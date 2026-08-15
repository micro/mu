package video

// Every thumbnail goes through Mu's origin, on every page that draws one.
//
// There are five places that render a thumbnail — the feed, the latest card,
// search results, the playlist view and the channel view — and four of them
// called thumbSrc. The channel and playlist views passed the URL the YouTube
// API returned straight into the img tag, so those two pages linked to
// i.ytimg.com while every other page proxied.
//
// That is not a cosmetic difference. It is the first reason the proxy exists,
// written at the top of thumbnail.go: a page of videos is two hundred images
// from one hostname, and anything that refuses that hostname — a content
// blocker, a filtering resolver, a network policy — breaks every image on the
// page at once. Which is exactly the report: fine everywhere, all broken on a
// channel.
//
// TestFeedDoesNotLinkToGooglesCDN was already here and could not catch it. It
// reads the cached feed and skips when there is none, and these two pages are
// built from a live API response at request time — there is no cached markup to
// inspect. So this reads the source instead: wherever an img is rendered, the
// value put in it has to have come from thumbSrc or ThumbURL.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// imgRender finds a rendered thumbnail tag in a format string.
var imgRender = regexp.MustCompile(`<img src="%s"`)

func TestEveryRenderedThumbnailIsProxied(t *testing.T) {
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	var checked int
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")

		for i, line := range lines {
			if !imgRender.MatchString(line) {
				continue
			}
			checked++

			// Look both ways. The value can be built inline in the argument
			// list, or assigned a few lines above — the feed and the latest card
			// both do the second, taking ThumbURL and falling back. Scanning
			// only forwards called those two broken when they are the pattern
			// being asked for.
			lo, hi := i-30, i+7
			if lo < 0 {
				lo = 0
			}
			if hi > len(lines) {
				hi = len(lines)
			}
			args := strings.Join(lines[lo:hi], "\n")

			if strings.Contains(args, "thumbSrc(") || strings.Contains(args, "ThumbURL(") {
				continue
			}
			t.Errorf("%s:%d renders a thumbnail without thumbSrc or ThumbURL — that "+
				"links straight to Google's CDN, so anything blocking i.ytimg.com "+
				"breaks every image on this page while the rest of /video is fine",
				f.Name(), i+1)
		}
	}

	if checked < 4 {
		t.Fatalf("found only %d thumbnail renders — this scan is broken, not the "+
			"code", checked)
	}
}

// And the proxy is what those calls resolve to, for a real video id.
func TestThumbSrcPrefersTheLocalPath(t *testing.T) {
	const id = "hE2HEj1JBcI"
	const remote = "https://i.ytimg.com/vi/" + id + "/mqdefault.jpg"

	if got := thumbSrc(id, remote); got != "/video/thumb?id="+id {
		t.Errorf("thumbSrc(%q, remote) = %q, want the local path", id, got)
	}

	// A channel or playlist id is not a video id, so there is no local
	// thumbnail to serve and the API's own URL is all there is. Falling back is
	// right; silently emitting an empty src would not be.
	const channel = "UCXuqSBlHAE6Xw-yeJA0Tunw"
	if got := thumbSrc(channel, remote); got != remote {
		t.Errorf("thumbSrc(channelID, remote) = %q, want the API URL", got)
	}
}
