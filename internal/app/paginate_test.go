package app

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func pagerFor(t *testing.T, query string, total, per int) Pager {
	t.Helper()
	return Paginate(httptest.NewRequest("GET", "/blog"+query, nil), total, per)
}

func TestAPageIsAWindowOnTheList(t *testing.T) {
	p := pagerFor(t, "", 55, 20)
	if p.Page != 1 || p.From != 0 || p.To != 20 || p.Pages != 3 {
		t.Fatalf("first page = %+v", p)
	}

	p = pagerFor(t, "?page=3", 55, 20)
	if p.From != 40 || p.To != 55 {
		t.Fatalf("last page = %+v, want the remaining 15", p)
	}
}

// A page number out of range is a stale bookmark or a link that outlived the
// posts behind it. Neither is worth an error, and neither may be a slice panic.
func TestAnImpossiblePageIsTheFirstOne(t *testing.T) {
	for _, q := range []string{"?page=0", "?page=-4", "?page=99", "?page=banana", "?page="} {
		p := pagerFor(t, q, 10, 20)
		if p.Page != 1 || p.From != 0 || p.To != 10 {
			t.Errorf("%s = %+v, want the first page", q, p)
		}
	}
}

func TestAnEmptyListPagesToNothing(t *testing.T) {
	p := pagerFor(t, "?page=2", 0, 20)
	if p.From != 0 || p.To != 0 || p.Pages != 1 {
		t.Fatalf("empty = %+v", p)
	}
	if p.Nav("/blog") != "" {
		t.Error("nothing to page through should show no pager")
	}
}

func TestOnePageShowsNoPager(t *testing.T) {
	if nav := pagerFor(t, "", 20, 20).Nav("/blog"); nav != "" {
		t.Errorf("a list that fits shows a pager: %q", nav)
	}
}

// A filter and a page number have to compose. Paging out of a tab that drops
// the tab is how a paginated page becomes worse than an unpaginated one.
func TestPagingKeepsTheRestOfTheQuery(t *testing.T) {
	p := pagerFor(t, "?tab=banned&page=2", 100, 20)
	nav := p.Nav("/admin/users")

	if !strings.Contains(nav, "tab=banned") {
		t.Errorf("paging dropped the filter: %s", nav)
	}
	if !strings.Contains(nav, "page=3") {
		t.Errorf("no link to the next page: %s", nav)
	}
	// Back to the first page is the bare path plus the filter — no page=1
	// hanging around to make two URLs for one page.
	if strings.Contains(nav, "page=1") {
		t.Errorf("the first page is spelled two ways: %s", nav)
	}
}
