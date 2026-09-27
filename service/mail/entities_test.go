package mail

import (
	"strings"
	"testing"
)

func TestMailPlainTextDecodesSmartPunctuation(t *testing.T) {
	got := stripHTMLTags(`<p>Here&rsquo;s today&rsquo;s plan &mdash; run &rarr; Arabic &amp; work.</p>`)
	if strings.TrimSpace(got) != "Here’s today’s plan — run → Arabic & work." {
		t.Fatal(got)
	}
}
