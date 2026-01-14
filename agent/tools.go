package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
	"mu/app"
	"mu/apps"
	"mu/data"
	"mu/news"
)

// VideoResult for agent responses
type VideoResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Channel   string `json:"channel"`
	Thumbnail string `json:"thumbnail"`
	URL       string `json:"url"`
}

// NewsResult for agent responses
type NewsResult struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Category    string `json:"category"`
}

// AppResult for agent responses
type AppResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	URL    string `json:"url"`
}

// videoSearch searches YouTube for videos
func (a *Agent) videoSearch(params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}
	
	app.Log("agent", "Video search: %s", query)
	
	// Use YouTube API directly
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		return &ToolResult{Success: false, Error: "YouTube API not configured"}, nil
	}
	
	client, err := youtube.NewService(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("YouTube client error: %v", err)}, nil
	}
	
	resp, err := client.Search.List([]string{"id", "snippet"}).
		Q(query).
		SafeSearch("strict").
		MaxResults(5).
		Type("video").
		Do()
	if err != nil {
		return &ToolResult{Success: false, Error: fmt.Sprintf("Search failed: %v", err)}, nil
	}
	
	var results []VideoResult
	for _, item := range resp.Items {
		if item.Id.VideoId == "" {
			continue
		}
		
		thumbnail := ""
		if item.Snippet.Thumbnails != nil && item.Snippet.Thumbnails.Medium != nil {
			thumbnail = item.Snippet.Thumbnails.Medium.Url
		}
		
		results = append(results, VideoResult{
			ID:        item.Id.VideoId,
			Title:     item.Snippet.Title,
			Channel:   item.Snippet.ChannelTitle,
			Thumbnail: thumbnail,
			URL:       fmt.Sprintf("/video?id=%s", item.Id.VideoId),
		})
	}
	
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data:    "No videos found",
		}, nil
	}
	
	// Build HTML preview
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<div class="agent-results video-results">`)
	for i, v := range results {
		if i == 0 {
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="video-result primary">
					<a href="%s"><img src="%s" alt="%s"></a>
					<div class="video-info">
						<a href="%s"><strong>%s</strong></a>
						<span class="channel">%s</span>
					</div>
				</div>`, v.URL, v.Thumbnail, v.Title, v.URL, v.Title, v.Channel))
		} else {
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="video-result">
					<a href="%s">%s</a> · %s
				</div>`, v.URL, v.Title, v.Channel))
		}
	}
	htmlBuilder.WriteString(`</div>`)
	
	return &ToolResult{
		Success: true,
		Data:    results,
		HTML:    htmlBuilder.String(),
	}, nil
}

// videoPlay returns a URL to play a specific video
func (a *Agent) videoPlay(params map[string]interface{}) (*ToolResult, error) {
	videoID, ok := params["video_id"].(string)
	if !ok || videoID == "" {
		return &ToolResult{Success: false, Error: "video_id is required"}, nil
	}
	
	app.Log("agent", "Video play: %s", videoID)
	
	url := fmt.Sprintf("/video?id=%s", videoID)
	
	return &ToolResult{
		Success: true,
		Data:    map[string]string{"url": url, "video_id": videoID},
		Action:  "navigate",
		URL:     url,
	}, nil
}

// newsSearch searches news articles
func (a *Agent) newsSearch(params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return &ToolResult{Success: false, Error: "query is required"}, nil
	}
	
	app.Log("agent", "News search: %s", query)
	
	// Use the data package's search
	results := data.Search(query, 5, data.WithType("news"))
	
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data:    "No news articles found",
		}, nil
	}
	
	var newsResults []NewsResult
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<div class="agent-results news-results">`)
	
	for i, entry := range results {
		// Get URL from metadata if available
		url := ""
		if entry.Metadata != nil {
			if u, ok := entry.Metadata["url"].(string); ok {
				url = u
			}
		}
		if url == "" {
			url = entry.ID
		}
		
		nr := NewsResult{
			Title:       entry.Title,
			Description: truncate(entry.Content, 150),
			URL:         url,
			Category:    entry.Type,
		}
		newsResults = append(newsResults, nr)
		
		if i == 0 {
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="news-result primary">
					<a href="/news?url=%s"><strong>%s</strong></a>
					<p>%s</p>
				</div>`, url, entry.Title, nr.Description))
		} else {
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="news-result">
					<a href="/news?url=%s">%s</a>
				</div>`, url, entry.Title))
		}
	}
	htmlBuilder.WriteString(`</div>`)
	
	return &ToolResult{
		Success: true,
		Data:    newsResults,
		HTML:    htmlBuilder.String(),
	}, nil
}

// newsRead gets full content of a news article
func (a *Agent) newsRead(params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok || url == "" {
		return &ToolResult{Success: false, Error: "url is required"}, nil
	}
	
	app.Log("agent", "News read: %s", url)
	
	// Search for the article by URL
	results := data.Search(url, 1, data.WithType("news"))
	
	if len(results) == 0 {
		return &ToolResult{
			Success: false,
			Error:   "Article not found",
		}, nil
	}
	
	article := results[0]
	
	// Get URL from metadata if available, otherwise use the input URL
	articleURL := url
	if article.Metadata != nil {
		if u, ok := article.Metadata["url"].(string); ok {
			articleURL = u
		}
	}
	if articleURL == "" {
		articleURL = article.ID
	}
	
	return &ToolResult{
		Success: true,
		Data: map[string]string{
			"title":   article.Title,
			"content": article.Content,
			"url":     articleURL,
		},
		HTML: fmt.Sprintf(`<div class="article-content">
			<h3>%s</h3>
			<p>%s</p>
			<a href="/news?url=%s">Read full article</a>
		</div>`, article.Title, truncate(article.Content, 500), articleURL),
	}, nil
}

// appCreate creates a new micro app
func (a *Agent) appCreate(params map[string]interface{}) (*ToolResult, error) {
	name, _ := params["name"].(string)
	description, _ := params["description"].(string)
	
	if name == "" || description == "" {
		return &ToolResult{Success: false, Error: "name and description are required"}, nil
	}
	
	app.Log("agent", "App create: %s", name)
	
	// Create the app asynchronously
	newApp, err := apps.CreateAppAsync(name, description, a.userID, a.userID)
	if err != nil {
		return &ToolResult{Success: false, Error: err.Error()}, nil
	}
	
	url := fmt.Sprintf("/apps/%s/develop", newApp.ID)
	
	return &ToolResult{
		Success: true,
		Data: AppResult{
			ID:     newApp.ID,
			Name:   newApp.Name,
			Status: newApp.Status,
			URL:    url,
		},
		Action: "navigate",
		URL:    url,
		HTML:   fmt.Sprintf(`<p>Creating app "%s"... <a href="%s">View progress</a></p>`, name, url),
	}, nil
}

// appModify modifies an existing app
func (a *Agent) appModify(params map[string]interface{}) (*ToolResult, error) {
	appID, _ := params["app_id"].(string)
	instruction, _ := params["instruction"].(string)
	
	if appID == "" || instruction == "" {
		return &ToolResult{Success: false, Error: "app_id and instruction are required"}, nil
	}
	
	app.Log("agent", "App modify: %s - %s", appID, instruction)
	
	// Get the app
	existingApp := apps.GetApp(appID)
	if existingApp == nil {
		return &ToolResult{Success: false, Error: "App not found"}, nil
	}
	
	// Check ownership
	if existingApp.AuthorID != a.userID {
		return &ToolResult{Success: false, Error: "Not authorized to modify this app"}, nil
	}
	
	// The modification is handled by the develop page
	// For now, return the URL to modify
	url := fmt.Sprintf("/apps/%s/develop", appID)
	
	return &ToolResult{
		Success: true,
		Data: map[string]string{
			"app_id":      appID,
			"instruction": instruction,
			"url":         url,
		},
		Action: "navigate",
		URL:    url,
		HTML:   fmt.Sprintf(`<p>Modify app at <a href="%s">%s</a></p>`, url, existingApp.Name),
	}, nil
}

// appList lists user's apps or searches public apps
func (a *Agent) appList(params map[string]interface{}) (*ToolResult, error) {
	query, _ := params["query"].(string)
	
	app.Log("agent", "App list: %s", query)
	
	var userApps []*apps.App
	if a.userID != "" {
		userApps = apps.GetUserApps(a.userID)
	}
	
	publicApps := apps.GetPublicApps()
	
	// Filter by query if provided
	var results []AppResult
	
	if query != "" {
		query = strings.ToLower(query)
		// Search in user apps
		for _, a := range userApps {
			if strings.Contains(strings.ToLower(a.Name), query) ||
				strings.Contains(strings.ToLower(a.Description), query) {
				results = append(results, AppResult{
					ID:   a.ID,
					Name: a.Name,
					URL:  fmt.Sprintf("/apps/%s", a.ID),
				})
			}
		}
		// Search in public apps
		for _, a := range publicApps {
			if strings.Contains(strings.ToLower(a.Name), query) ||
				strings.Contains(strings.ToLower(a.Description), query) {
				results = append(results, AppResult{
					ID:   a.ID,
					Name: a.Name + " (public)",
					URL:  fmt.Sprintf("/apps/%s", a.ID),
				})
			}
		}
	} else {
		// Return user's apps
		for _, a := range userApps {
			results = append(results, AppResult{
				ID:   a.ID,
				Name: a.Name,
				URL:  fmt.Sprintf("/apps/%s", a.ID),
			})
		}
	}
	
	if len(results) == 0 {
		return &ToolResult{
			Success: true,
			Data:    "No apps found",
		}, nil
	}
	
	// Build HTML
	var htmlBuilder strings.Builder
	htmlBuilder.WriteString(`<div class="agent-results app-results"><ul>`)
	for _, r := range results {
		htmlBuilder.WriteString(fmt.Sprintf(`<li><a href="%s">%s</a></li>`, r.URL, r.Name))
	}
	htmlBuilder.WriteString(`</ul></div>`)
	
	return &ToolResult{
		Success: true,
		Data:    results,
		HTML:    htmlBuilder.String(),
	}, nil
}

// marketPrice gets market prices
func (a *Agent) marketPrice(params map[string]interface{}) (*ToolResult, error) {
	symbol, _ := params["symbol"].(string)
	if symbol == "" {
		return &ToolResult{Success: false, Error: "symbol is required"}, nil
	}
	
	app.Log("agent", "Market price: %s", symbol)
	
	// Get cached prices from news package
	prices := news.GetAllPrices()
	
	symbol = strings.ToUpper(symbol)
	
	// Try to find the price
	price, found := prices[symbol]
	if !found {
		// Try with common prefixes
		for k, v := range prices {
			if strings.Contains(strings.ToUpper(k), symbol) {
				price = v
				symbol = k
				found = true
				break
			}
		}
	}
	
	if !found {
		return &ToolResult{
			Success: true,
			Data:    fmt.Sprintf("Price not found for %s. Available: %v", symbol, getAvailableSymbols(prices)),
		}, nil
	}
	
	return &ToolResult{
		Success: true,
		Data: map[string]interface{}{
			"symbol": symbol,
			"price":  price,
		},
		HTML: fmt.Sprintf(`<p><strong>%s</strong>: $%.2f</p>`, symbol, price),
	}, nil
}

// Helper functions

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getAvailableSymbols(prices map[string]float64) []string {
	var symbols []string
	for k := range prices {
		symbols = append(symbols, k)
	}
	return symbols
}
