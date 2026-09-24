package app

import (
	"mu/internal/auth"
	"strings"
	"testing"
)

func TestAppShellKeepsPublicFooterOutOfSignedInPages(t *testing.T) {
	for _, path := range []string{"/home", "/docs", "/mail", "/chat", "/sms", "/work"} {
		page := ConsoleHTML("Page", "<p>Content</p>", &auth.Account{ID: "shell-reader"}, path)
		if strings.Contains(page, `aria-label="Site information"`) {
			t.Fatalf("website footer on %s", path)
		}
		if !strings.Contains(page, `class="brand">Micro</a>`) || !strings.Contains(page, `aria-label="Main navigation"`) {
			t.Fatalf("missing app identity/navigation on %s", path)
		}
	}
	page := ConsoleHTML("About", "<p>About Micro</p>", nil, "/about")
	if !strings.Contains(page, `aria-label="Site information"`) {
		t.Fatal("public website lost its footer")
	}
}

func TestPublicPagesRetainFooterForSignedInReaders(t *testing.T) {
	for _, path := range []string{"/", "/about", "/contact", "/pricing", "/privacy", "/status", "/blog", "/blog/post?id=123"} {
		page := ConsoleHTML("Public page", "<p>Content</p>", &auth.Account{ID: "reader"}, path)
		if !strings.Contains(page, `aria-label="Site information"`) {
			t.Fatalf("missing public footer on %s", path)
		}
	}
}

func TestSignedInShellPreservesContentLayoutClasses(t *testing.T) {
	page := renderShell("en", "Article", "", "reading-page editorial-reading", "<p>Article</p>", &auth.Account{ID: "reader"}, "/blog", "/blog")
	if !strings.Contains(page, `class="document-page reading-page editorial-reading signed-in"`) {
		t.Fatal("lost content or account layout classes")
	}
	if !strings.Contains(page, `class="runtime-brand" href="/home">Micro</a>`) {
		t.Fatal("missing desktop sidebar brand")
	}
}
