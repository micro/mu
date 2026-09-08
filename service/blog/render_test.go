package blog

import (
	"strings"
	"testing"
)

func TestLinkifyProtectsCurrencyDollarsFromMathRendering(t *testing.T) {
	got := Linkify("Daily Digest: AI startup raised $1 billion while BTC traded at $94,000.")
	for _, bad := range []string{"$1", "$94"} {
		if strings.Contains(got, bad) {
			t.Fatalf("Linkify left currency sequence %q exposed to math rendering: %q", bad, got)
		}
	}
	for _, want := range []string{"$\u20601 billion", "$\u206094,000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Linkify() missing protected currency %q in %q", want, got)
		}
	}
}

func TestInlineVideoCitationsRemainReadableLinks(t *testing.T) {
	got := Linkify(`Tehran warned of a [faster response](https://www.youtube.com/watch?v=Q_QEtgYTbfc), while [another report](https://youtu.be/58hVkLDMPdY) followed.`)
	for _, want := range []string{`href="https://www.youtube.com/watch?v=Q_QEtgYTbfc"`, `>faster response</a>, while`, `>another report</a> followed.`} {
		if !strings.Contains(got, want) {
			t.Errorf("lost citation %q: %s", want, got)
		}
	}
	if strings.Contains(got, "iframe") || strings.Contains(got, "/video?id=") {
		t.Fatal("post embedded the video page")
	}
	if got := Linkify(`<iframe src="https://example.com"></iframe>`); strings.Contains(got, "<iframe") {
		t.Fatal("raw iframe injected")
	}
}
