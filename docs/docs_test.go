package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocument_Structure(t *testing.T) {
	doc := Document{
		Slug:        "test-doc",
		Filename:    "TEST.md",
		Title:       "Test Document",
		Description: "A test doc",
		Category:    "Testing",
	}
	if doc.Slug != "test-doc" {
		t.Error("expected slug")
	}
	if doc.Filename != "TEST.md" {
		t.Error("expected filename")
	}
}

func TestCatalog_NotEmpty(t *testing.T) {
	if len(catalog) == 0 {
		t.Error("catalog should not be empty")
	}
}

func TestCatalog_UniqueSlugs(t *testing.T) {
	seen := make(map[string]bool)
	for _, doc := range catalog {
		if seen[doc.Slug] {
			t.Errorf("duplicate slug: %q", doc.Slug)
		}
		seen[doc.Slug] = true
	}
}

func TestCatalog_AllFieldsPopulated(t *testing.T) {
	for _, doc := range catalog {
		if doc.Slug == "" {
			t.Errorf("doc %q has empty slug", doc.Title)
		}
		if doc.Filename == "" {
			t.Errorf("doc %q has empty filename", doc.Slug)
		}
		if doc.Title == "" {
			t.Errorf("doc %q has empty title", doc.Slug)
		}
		if doc.Description == "" {
			t.Errorf("doc %q has empty description", doc.Slug)
		}
		if doc.Category == "" {
			t.Errorf("doc %q has empty category", doc.Slug)
		}
	}
}

func TestCatalog_HasAboutDoc(t *testing.T) {
	found := false
	for _, doc := range catalog {
		if doc.Slug == "about" {
			found = true
			if doc.Filename != "ABOUT.md" {
				t.Errorf("about doc filename should be ABOUT.md, got %q", doc.Filename)
			}
			break
		}
	}
	if !found {
		t.Error("catalog should contain 'about' document")
	}
}

// The markdown files open with their own H1 because they are read on GitHub
// too. The page shell renders doc.Title above the content, so a served doc that
// keeps its H1 shows its name twice — which is exactly what /help/about,
// /help/installation and /help/mcp did.
func TestServedDocDoesNotRepeatItsTitle(t *testing.T) {
	for _, doc := range catalog {
		req := httptest.NewRequest(http.MethodGet, "/help/"+doc.Slug, nil)
		w := httptest.NewRecorder()
		Handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("%s: status %d", doc.Slug, w.Code)
			continue
		}
		body := w.Body.String()
		i := strings.Index(body, `<div class="docs-content">`)
		if i < 0 {
			t.Errorf("%s: no docs-content block", doc.Slug)
			continue
		}
		if strings.Contains(body[i:], "<h1>") {
			t.Errorf("%s: content starts with an H1, repeating the page title %q", doc.Slug, doc.Title)
		}
	}
}

func TestStripTitle(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"leading h1", "# Installation\n\nBody\n", "Body\n"},
		{"blank lines first", "\n\n# CLI\nBody\n", "Body\n"},
		{"no h1", "Body only\n", "Body only\n"},
		{"h1 later stays", "Intro\n\n# Not the title\n", "Intro\n\n# Not the title\n"},
		{"bold opener untouched", "**Guiding principles**\n\nBody\n", "**Guiding principles**\n\nBody\n"},
		{"h1 only", "# Alone", ""},
	} {
		if got := string(stripTitle([]byte(tc.in))); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestCatalog_HasCategories(t *testing.T) {
	categories := make(map[string]bool)
	for _, doc := range catalog {
		categories[doc.Category] = true
	}
	// Two categories now: get connected, then look things up. Features and
	// Developer went with the docs that were folded into Architecture.
	expected := []string{"Getting Started", "Reference"}
	for _, cat := range expected {
		if !categories[cat] {
			t.Errorf("expected category %q in catalog", cat)
		}
	}
}
