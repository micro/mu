package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mu/internal/settings"
	"net/http"
	"net/url"
	"strings"
	"time"
)

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

var imageSearchClient = &http.Client{Timeout: 12 * time.Second}
var imageSearchURL = "https://api.search.brave.com/res/v1/images/search"

func Search(ctx context.Context, query string) ([]WebImage, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 400 || len(strings.Fields(query)) > 50 {
		return nil, fmt.Errorf("use a search of up to 400 characters and 50 words")
	}
	key := settings.Get("BRAVE_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("web image search is not configured on this instance")
	}
	q := url.Values{"q": {query}, "count": {"24"}, "safesearch": {"strict"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, imageSearchURL+"?"+q.Encode(), nil)
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
