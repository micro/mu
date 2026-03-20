package docs

import (
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

func TestCatalog_HasCategories(t *testing.T) {
	categories := make(map[string]bool)
	for _, doc := range catalog {
		categories[doc.Category] = true
	}
	expected := []string{"Getting Started", "Features", "Reference", "Developer"}
	for _, cat := range expected {
		if !categories[cat] {
			t.Errorf("expected category %q in catalog", cat)
		}
	}
}
