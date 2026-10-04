package agent

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"mu/internal/origin"
	"mu/service/web"
)

var readingURL = regexp.MustCompile(`https?://[^\s<>"\x60)\]]+`)

// Keep article identity while dropping fragments and common tracking parameters.
func readingSourceKey(raw string) string {
	u, err := url.Parse(strings.TrimRight(raw, ".,;!"))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	local, _ := url.Parse(origin.Self())
	if local != nil && strings.EqualFold(local.Hostname(), u.Hostname()) {
		return ""
	}
	u.Scheme = "https"
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(strings.ToLower(k), "utm_") || k == "fbclid" || k == "gclid" {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func readingSourceURLs(text string) []string {
	var urls []string
	seen := map[string]bool{}
	for _, raw := range readingURL.FindAllString(text, -1) {
		key := readingSourceKey(raw)
		if key != "" && !seen[key] {
			urls = append(urls, key)
			seen[key] = true
		}
	}
	return urls
}

// Prefer new publishers, but permit a different article from a familiar publisher.
// Never silently recycle an article in the recent reading history.
func freshReadingSources(items []web.BraveResult, recent, instructions string) []web.BraveResult {
	used, hosts, pinned := map[string]bool{}, map[string]int{}, map[string]bool{}
	for _, raw := range readingSourceURLs(recent) {
		used[raw] = true
		u, _ := url.Parse(raw)
		hosts[u.Hostname()]++
	}
	for _, raw := range readingSourceURLs(instructions) {
		pinned[raw] = true
	}
	var candidates []web.BraveResult
	seen := map[string]bool{}
	for _, item := range items {
		key := readingSourceKey(item.URL)
		if key == "" || seen[key] || used[key] && !pinned[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, item)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, _ := url.Parse(readingSourceKey(candidates[i].URL))
		b, _ := url.Parse(readingSourceKey(candidates[j].URL))
		return hosts[a.Hostname()] < hosts[b.Hostname()]
	})
	var selected, extra []web.BraveResult
	chosen := map[string]bool{}
	for _, item := range candidates {
		u, _ := url.Parse(readingSourceKey(item.URL))
		if chosen[u.Hostname()] {
			extra = append(extra, item)
			continue
		}
		chosen[u.Hostname()] = true
		selected = append(selected, item)
	}
	return append(selected, extra...)
}
