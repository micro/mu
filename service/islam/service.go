// Package islam provides source material from Aslam's Islamic knowledge base.
package islam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mu/internal/app"
	"mu/internal/service"
)

var knowledgeBase = "https://aslam.org"
var knowledgeClient = &http.Client{Timeout: 15 * time.Second}

// Server reads published sources; it does not call Aslam's chat model.
type Server struct{}

type SearchRequest struct {
	Query      string `json:"query" required:"true" description:"Keywords to find in the Islamic knowledge base"`
	Collection string `json:"collection" description:"Optional source: quran, hadith, names, seerah, ghazali, islamqa, adhkar or salihin"`
	Limit      int    `json:"limit" description:"Maximum results, 1-50 (default 20)"`
}

type Result struct {
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Role    string `json:"role"`
	Source  string `json:"source"`
	URL     string `json:"url"`
}

type SearchResponse struct {
	Results []Result `json:"results" description:"Source excerpts with citation links. Use Read for the complete passage before quoting or interpreting it"`
}

type ReadRequest struct {
	Path string `json:"path" required:"true" description:"Resource path from a search result, e.g. /quran/2/255 or /seerah/page/10; an https://aslam.org URL is also accepted"`
}

type ReadResponse struct {
	Kind     string                 `json:"kind"`
	Source   string                 `json:"source"`
	URL      string                 `json:"url"`
	Resource map[string]interface{} `json:"resource" description:"Complete source record, preserving Arabic, translation, commentary and reference metadata separately where available"`
}

var collections = map[string]bool{"quran": true, "hadith": true, "names": true, "seerah": true, "ghazali": true, "islamqa": true, "adhkar": true, "salihin": true}

func get(ctx context.Context, path string, query url.Values, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, knowledgeBase+path+"?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := knowledgeClient.Do(req)
	if err != nil {
		return fmt.Errorf("Aslam knowledge API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Aslam knowledge API returned status %d", resp.StatusCode)
	}
	const maxBody = 4 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("Aslam knowledge response is too large")
	}
	if err = json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("invalid Aslam knowledge response: %w", err)
	}
	return nil
}

// Search finds passages in Aslam's published reference collections.
// @example {"query":"patience", "collection":"quran"}
func (Server) Search(ctx context.Context, req *SearchRequest, rsp *SearchResponse) error {
	q := strings.TrimSpace(req.Query)
	if q == "" || len(q) > 1000 {
		return fmt.Errorf("query must contain 1-1000 bytes")
	}
	if req.Collection != "" && !collections[req.Collection] {
		return fmt.Errorf("unknown collection")
	}
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return fmt.Errorf("limit must be between 1 and 50")
	}
	var result SearchResponse
	if err := get(ctx, "/api/knowledge/search", url.Values{"q": {q}, "collection": {req.Collection}, "limit": {fmt.Sprint(limit)}}, &result); err != nil {
		return err
	}
	if result.Results == nil {
		return fmt.Errorf("Aslam knowledge response is missing results")
	}
	rsp.Results = []Result{}
	for _, item := range result.Results {
		if !collections[item.Kind] || (req.Collection != "" && item.Kind != req.Collection) {
			continue
		}
		path, err := resourcePath(item.URL)
		if err != nil {
			return fmt.Errorf("invalid source reference from Aslam")
		}
		item.URL = "https://aslam.org" + path
		rsp.Results = append(rsp.Results, item)
		if len(rsp.Results) == limit {
			break
		}
	}
	return nil
}

func resourcePath(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid resource path")
	}
	if u.Host != "" && (u.Scheme != "https" || u.Host != "aslam.org") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("resource must be an Aslam knowledge path")
	}
	if u.Scheme != "" && u.Host == "" {
		return "", fmt.Errorf("invalid resource URL")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if !strings.HasPrefix(u.Path, "/") || len(parts) < 2 || !collections[parts[0]] {
		return "", fmt.Errorf("unknown knowledge resource")
	}
	for _, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", fmt.Errorf("invalid resource path")
		}
	}
	return u.Path, nil
}

// Read retrieves a complete reference passage from a Search result.
// @example {"path":"/quran/2/255"}
func (Server) Read(ctx context.Context, req *ReadRequest, rsp *ReadResponse) error {
	path, err := resourcePath(req.Path)
	if err != nil {
		return err
	}
	if err = get(ctx, "/api/knowledge/resource", url.Values{"path": {path}}, rsp); err != nil {
		return err
	}
	if !collections[rsp.Kind] || rsp.Resource == nil || !strings.HasPrefix(path, "/"+rsp.Kind+"/") {
		return fmt.Errorf("invalid Aslam knowledge resource")
	}
	rsp.URL = "https://aslam.org" + path
	return nil
}

var Spec = service.Spec{
	Name: "islam", Handler: new(Server), Description: "Search Islamic knowledge from Aslam.org",
	Endpoints: map[string]service.Endpoint{
		"Search": {Doc: "Search Aslam.org's Quran, hadith, names of Allah, Seerah, Ghazali, IslamQA, adhkar and Riyad us-Salihin. Returns reference excerpts, not generated answers. Distinguish Quran and hadith from biography and scholarly interpretation; cite the source."},
		"Read":   {Doc: "Read the full Islamic source passage from an islam_search result, with its source and reference. Preserve distinctions between original text, translation and commentary."},
	},
}

func Load() {
	if err := service.Register(Spec); err != nil {
		app.Log("islam", "service register failed: %v", err)
	}
}
