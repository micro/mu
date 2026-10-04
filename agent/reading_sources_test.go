package agent

import (
	"mu/service/web"
	"strings"
	"testing"
)

func TestReadingSourcesAvoidRecentArticlesAndVaryPublishers(t *testing.T) {
	recent := strings.Repeat("Earlier reading. ", 100) + "[Source](http://www.example.org/old/?utm_source=email#part)"
	items := []web.BraveResult{
		{URL: "https://example.org/old"},
		{URL: "https://example.org/new"},
		{URL: "https://new.org/one"},
		{URL: "https://new.org/two"},
		{URL: "https://other.org/article"},
		{URL: "https://new.org/one?utm_medium=email"},
	}
	got := freshReadingSources(items, recent, "")
	want := []string{"https://new.org/one", "https://other.org/article", "https://example.org/new", "https://new.org/two"}
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i := range want {
		if got[i].URL != want[i] {
			t.Fatalf("%+v", got)
		}
	}
	if got := freshReadingSources(items[:1], recent, ""); len(got) != 0 {
		t.Fatal("recycled old article")
	}
	if got := freshReadingSources(items[:1], recent, "Continue studying https://example.org/old"); len(got) != 1 {
		t.Fatal("ignored explicitly requested source")
	}
}

func TestReadingSourceIdentityPreservesMeaningfulQueries(t *testing.T) {
	a := readingSourceKey("https://example.org/read?id=1&utm_source=x#p")
	b := readingSourceKey("http://www.example.org/read?id=1")
	if a != b {
		t.Fatalf("%s != %s", a, b)
	}
	if a == readingSourceKey("https://example.org/read?id=2") {
		t.Fatal("merged different articles")
	}
}
