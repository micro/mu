package app

import (
	"strings"
	"testing"
)

// Render handles content authored by users and by remote servers (blog posts
// and comments, federated ActivityPub, model output). None of it may produce
// executable markup.
func TestRenderStripsActiveContent(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		absent  []string
		present []string
	}{
		{
			name:   "script tag",
			in:     `<script>alert(document.cookie)</script>`,
			absent: []string{"<script"},
		},
		{
			name:   "img onerror",
			in:     `<img src=x onerror="alert(1)">`,
			absent: []string{"onerror", "<img src=x"},
		},
		{
			name:   "iframe",
			in:     `text <iframe src="//evil.example"></iframe>`,
			absent: []string{"<iframe"},
		},
		{
			name:   "inline event handler",
			in:     `<div onclick="alert(1)">hi</div>`,
			absent: []string{"onclick", "<div"},
		},
		{
			name:   "javascript link",
			in:     `[click](javascript:alert(1))`,
			absent: []string{`href="javascript:`},
		},
		{
			name:   "vbscript link",
			in:     `[click](vbscript:alert(1))`,
			absent: []string{`href="vbscript:`},
		},
		{
			name:   "javascript image",
			in:     `![x](javascript:alert(1))`,
			absent: []string{"javascript:"},
		},
		{
			name:   "data uri image",
			in:     `![x](data:text/html;base64,PHNjcmlwdD4=)`,
			absent: []string{"data:text/html"},
		},
		{
			name:   "scheme obfuscated with control chars",
			in:     "![x](java\tscript:alert(1))",
			absent: []string{"script:alert"},
		},
		{
			name:   "svg with script",
			in:     `<svg><script>alert(1)</script></svg>`,
			absent: []string{"<svg", "<script"},
		},
		{
			name:   "style tag",
			in:     `<style>body{background:url(javascript:alert(1))}</style>`,
			absent: []string{"<style"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := string(Render([]byte(tc.in)))
			lower := strings.ToLower(out)
			for _, bad := range tc.absent {
				if strings.Contains(lower, strings.ToLower(bad)) {
					t.Errorf("rendered output still contains %q\ninput:  %s\noutput: %s", bad, tc.in, out)
				}
			}
			for _, want := range tc.present {
				if !strings.Contains(out, want) {
					t.Errorf("rendered output missing %q\noutput: %s", want, out)
				}
			}
		})
	}
}

// The safety flags must not cost us ordinary markdown.
func TestRenderKeepsLegitimateMarkdown(t *testing.T) {
	in := "# Title\n\nSome **bold** and *italic* and `code`.\n\n" +
		"- one\n- two\n\n> quoted\n\n[link](https://example.com)\n\n" +
		"![pic](https://example.com/a.png)\n\n[rel](/blog/post)\n\n" +
		"[mail](mailto:a@example.com)\n\n```\nfenced\n```\n\n" +
		"| a | b |\n|---|---|\n| 1 | 2 |\n"

	out := string(Render([]byte(in)))
	for _, want := range []string{
		"<h1", "<strong>bold</strong>", "<em>italic</em>", "<code>code</code>",
		"<ul>", "<li>one</li>", "<blockquote>",
		`href="https://example.com"`, `src="https://example.com/a.png"`,
		`href="/blog/post"`, `href="mailto:a@example.com"`,
		"<table>", "<pre>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output\ngot: %s", want, out)
		}
	}
}

// Repo-shipped docs deliberately embed HTML and must keep working.
func TestRenderTrustedKeepsHTML(t *testing.T) {
	out := string(RenderTrusted([]byte(`<div class="note">hello</div>`)))
	if !strings.Contains(out, `<div class="note">`) {
		t.Errorf("trusted render dropped HTML: %s", out)
	}
}

func TestSafeURL(t *testing.T) {
	safe := []string{
		"", "/img/a.png", "a.png", "../a.png", "https://x/a.png",
		"HTTP://x/a.png", "//cdn.example/a.png", "?q=a:b", "#frag",
	}
	unsafe := []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "vbscript:alert(1)",
		"data:text/html,<script>", "java\tscript:alert(1)",
		" javascript:alert(1)", "file:///etc/passwd",
	}
	for _, s := range safe {
		if !safeURL(s) {
			t.Errorf("safeURL(%q) = false, want true", s)
		}
	}
	for _, s := range unsafe {
		if safeURL(s) {
			t.Errorf("safeURL(%q) = true, want false", s)
		}
	}
}

// Page titles and descriptions come from query strings and stored records and
// land in <title>, a meta attribute and an <h1>.
func TestRenderHTMLEscapesTitleAndDescription(t *testing.T) {
	out := RenderHTML(`x" onload="alert(1)`, `</title><script>alert(1)</script>`, "<p>body</p>")

	if strings.Contains(out, `onload="alert(1)`) {
		t.Error("title escaped out of its attribute context")
	}
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("description injected a script tag")
	}
	if !strings.Contains(out, "<p>body</p>") {
		t.Error("body should not be escaped — handlers pass HTML")
	}
}
