package app

// One window over a long list.
//
// A page that renders everything gets slower every day it is used, and the
// instance it is slowest on is the one that has been running longest — which is
// to say the one somebody actually depends on. /blog joined every post ever
// written into one string and served it; /social did the same with every
// thread. Neither had a limit of any kind, and both are append-only.
//
// This is the whole of the convention, so that two pages that page do it the
// same way: ?page=N, 1-based, newest first, with the rest of the query string
// carried along — a filter and a page number have to compose, or paging out of
// a tab silently drops the tab.

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// PerPage is what a list page shows when it has no reason to differ.
const PerPage = 20

// Pager is which slice of a list this request asked for.
type Pager struct {
	Page  int // 1-based
	Pages int
	From  int // index of the first item shown
	To    int // index just past the last
	Total int

	query url.Values
}

// Paginate reads ?page from the request and clamps it to what exists.
//
// Out of range is the first page rather than an error: a page number is
// somebody's stale bookmark or a link that outlived the posts behind it, and
// neither is worth a 404.
func Paginate(r *http.Request, total, per int) Pager {
	if per <= 0 {
		per = PerPage
	}
	pages := (total + per - 1) / per
	if pages < 1 {
		pages = 1
	}

	n, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if n < 1 || n > pages {
		n = 1
	}

	from := (n - 1) * per
	to := from + per
	if to > total {
		to = total
	}
	if from > total {
		from = total
	}

	q := url.Values{}
	for k, v := range r.URL.Query() {
		if k != "page" {
			q[k] = v
		}
	}
	return Pager{Page: n, Pages: pages, From: from, To: to, Total: total, query: q}
}

// Nav is the older/newer pair, or nothing at all when everything fits.
//
// Older on the left because that is the direction of reading a list that starts
// with the newest thing: the interesting move is backwards in time, and it is
// the one that should be under the thumb.
func (p Pager) Nav(path string) string {
	if p.Pages <= 1 {
		return ""
	}

	link := func(n int, label string) string {
		q := url.Values{}
		for k, v := range p.query {
			q[k] = v
		}
		if n > 1 {
			q.Set("page", strconv.Itoa(n))
		}
		href := path
		if s := q.Encode(); s != "" {
			href += "?" + s
		}
		return fmt.Sprintf(`<a href="%s" class="pager-link">%s</a>`, href, label)
	}

	var older, newer string
	if p.Page < p.Pages {
		older = link(p.Page+1, "Older →")
	}
	if p.Page > 1 {
		newer = link(p.Page-1, "← Newer")
	}

	return fmt.Sprintf(`<nav class="pager">%s<span class="pager-at">%d of %d</span>%s</nav>%s`,
		newer, p.Page, p.Pages, older, PagerCSS)
}

// PagerCSS is emitted with the nav rather than kept in the stylesheet, so a
// page gains pagination by calling one function.
const PagerCSS = `<style>
.pager{display:flex;align-items:center;justify-content:space-between;gap:12px;margin:24px 0 8px;padding-top:16px;border-top:1px solid #eee}
.pager-link{font-size:14px;color:#555;text-decoration:none}
.pager-link:hover{color:#000}
.pager-at{font-size:12px;color:#aaa;margin:0 auto}
</style>`
