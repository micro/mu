package images

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/quota"
	"mu/internal/settings"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WebSearchRequest struct {
	Query string `json:"query" required:"true" description:"Images to find on the web"`
}
type WebImage struct {
	Title     string `json:"title"`
	URL       string `json:"url"`
	Source    string `json:"source"`
	Thumbnail struct {
		Src string `json:"src"`
	} `json:"thumbnail"`
	Properties struct {
		URL string `json:"url"`
	} `json:"properties"`
}
type WebSearchResponse struct {
	Results []WebImage `json:"results"`
}

var imageSearchClient = &http.Client{Timeout: 12 * time.Second}
var imageSearchURL = "https://api.search.brave.com/res/v1/images/search"

func webImages(query string) ([]WebImage, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 400 || len(strings.Fields(query)) > 50 {
		return nil, fmt.Errorf("use a search of up to 400 characters and 50 words")
	}
	key := settings.Get("BRAVE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("web image search is not configured on this instance")
	}
	q := url.Values{"q": {query}, "count": {"24"}, "safesearch": {"strict"}}
	req, _ := http.NewRequest(http.MethodGet, imageSearchURL+"?"+q.Encode(), nil)
	req.Header.Set("X-Subscription-Token", key)
	req.Header.Set("Accept", "application/json")
	resp, err := imageSearchClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach image search")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("image search is unavailable (%d)", resp.StatusCode)
	}
	var result struct {
		Results []WebImage
		Extra   struct {
			MightBeOffensive bool `json:"might_be_offensive"`
		}
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not read image search results")
	}
	if result.Extra.MightBeOffensive {
		return nil, fmt.Errorf("these results were filtered; try another search")
	}
	var safe []WebImage
	for _, r := range result.Results {
		if webURL(r.URL) && webURL(r.Thumbnail.Src) && webURL(r.Properties.URL) {
			safe = append(safe, r)
		}
		if len(safe) == 24 {
			break
		}
	}
	return safe, nil
}
func webURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http")
}
func (Server) WebSearch(ctx context.Context, req *WebSearchRequest, rsp *WebSearchResponse) error {
	var err error
	rsp.Results, err = webImages(req.Query)
	return err
}
func webSearchPage(w http.ResponseWriter, r *http.Request) {
	owner, ok := app.BillableCaller(w, r, quota.OpWebSearch)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !auth.StrictCSRF(r) {
		app.Forbidden(w, r, "Invalid CSRF token")
		return
	}
	if err := auth.CheckPostRate(owner); err != nil {
		app.RespondError(w, 429, "Please wait before another search.")
		return
	}
	results, err := webImages(r.PostFormValue("query"))
	if err == nil {
		err = quota.Charge(owner, quota.OpWebSearch, nil)
	}
	b := `<p><a href="/images">← Images</a></p>`
	if err != nil {
		b += `<p role="alert">` + html.EscapeString(err.Error()) + `</p>`
	} else {
		b += `<div class="thumb-grid">`
		for _, x := range results {
			b += `<figure class="m-0"><a href="` + html.EscapeString(x.URL) + `" target="_blank" rel="noopener noreferrer"><img class="w-full rounded-lg" src="` + html.EscapeString(x.Thumbnail.Src) + `" alt="` + html.EscapeString(x.Title) + `" loading="lazy" referrerpolicy="no-referrer"></a><figcaption class="text-sm">` + html.EscapeString(x.Title) + `<div class="text-muted">` + html.EscapeString(x.Source) + `</div></figcaption></figure>`
		}
		b += `</div>`
		if len(results) == 0 {
			b += `<p>No matching images.</p>`
		}
	}
	app.Respond(w, r, app.Response{Title: "Images", HTML: b})
}
