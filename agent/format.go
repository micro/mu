package agent

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

func formatToolResult(toolName, result string, args map[string]any) string {
	switch toolName {
	case "news", "news_search", "news_headlines":
		return withCurrentDateContext(formatNewsResult(result))
	case "news_list", "news_read":
		return withCurrentDateContext(result)
	case "video_search":
		return formatVideoResult(result)
	case "prayer_reflection", "islam_today", "islam":
		return formatReminderResult(result)
	case "search":
		return withCurrentDateContext(formatSearchResult(result))
	case "web_search", "search_web", "weather_forecast":
		return withCurrentDateContext(result)
	case "markets", "markets_list":
		return withCurrentDateContext(formatMarketsResult(result))
	case "web_fetch", "search_fetch":
		return formatWebFetchResult(result)
	case "places_search", "places_nearby":
		return formatPlacesResult(result, args)
	case "wallet_balance":
		return formatWalletBalanceResult(result)
	case "apps_search":
		return formatAppsSearchResult(result)
	case "apps_read":
		return formatAppsReadResult(result)
	case "apps_build":
		return formatAppsBuildResult(result)
	case "apps_edit":
		return formatAppsBuildResult(result) // same format: returns app details
	}
	return plainToolText(result)
}

func plainToolText(result string) string {
	trimmed := strings.TrimSpace(result)
	if !strings.HasPrefix(trimmed, "{") {
		return result
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &body); err != nil {
		return result
	}
	raw, ok := body["text"]
	if !ok || len(body) != 1 {
		return result
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return result
	}
	if strings.TrimSpace(text) == "" {
		return result
	}
	return text
}

func formatMarketsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		return strings.TrimSpace(textData.Text)
	}

	var data struct {
		Category  string `json:"category"`
		UpdatedAt string `json:"updated_at"`
		Stale     bool   `json:"stale"`
		Partial   bool   `json:"partial"`
		Freshness string `json:"freshness"`
		Data      []struct {
			Symbol    string  `json:"symbol"`
			Price     float64 `json:"price"`
			Change24h float64 `json:"change_24h"`
			Type      string  `json:"type"`
			UpdatedAt string  `json:"updated_at"`
			Source    string  `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Data) == 0 {
		return "No market prices available right now."
	}

	category := strings.TrimSpace(data.Category)
	if category == "" {
		category = "market"
	}

	var b strings.Builder
	if strings.TrimSpace(data.Freshness) != "" {
		fmt.Fprintf(&b, "%s.\n", strings.TrimSuffix(strings.TrimSpace(data.Freshness), "."))
	} else if strings.TrimSpace(data.UpdatedAt) != "" {
		fmt.Fprintf(&b, "Last refresh: %s.\n", strings.TrimSpace(data.UpdatedAt))
	}
	if data.Stale {
		b.WriteString("Disclosure: market data may be stale.\n")
	}
	if data.Partial {
		b.WriteString("Disclosure: some requested symbols are unavailable from the current source.\n")
	}
	fmt.Fprintf(&b, "Live %s prices:\n", category)
	count := 0
	for _, item := range data.Data {
		symbol := strings.TrimSpace(item.Symbol)
		if symbol == "" || item.Price == 0 {
			continue
		}
		if item.Change24h != 0 {
			fmt.Fprintf(&b, "%s: $%s (%+.2f%% 24h)\n", symbol, formatMarketPrice(item.Price), item.Change24h)
		} else {
			fmt.Fprintf(&b, "%s: $%s\n", symbol, formatMarketPrice(item.Price))
		}
		count++
	}
	if count == 0 {
		return fmt.Sprintf("No %s prices available right now.", category)
	}
	return strings.TrimSpace(b.String())
}

func formatMarketPrice(price float64) string {
	switch {
	case price >= 100:
		return fmt.Sprintf("%.2f", price)
	case price >= 1:
		return fmt.Sprintf("%.3f", price)
	default:
		return fmt.Sprintf("%.6f", price)
	}
}

func formatNewsResult(result string) string {
	var textData struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &textData); err == nil && strings.TrimSpace(textData.Text) != "" {
		result = strings.TrimSpace(textData.Text)
	}

	var data struct {
		Items []formattedNewsItem `json:"items"`
		Feed  []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			Published   string `json:"published"`
			PostedAt    string `json:"posted_at"`
		} `json:"feed"`
		Results []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Category    string `json:"category"`
			URL         string `json:"url"`
			PostedAt    string `json:"posted_at"`
		} `json:"results"`
		Freshness struct {
			Status           string `json:"status"`
			Notice           string `json:"notice"`
			RequestedDate    string `json:"requested_date"`
			FreshestPostedAt string `json:"freshest_posted_at"`
		} `json:"freshness"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	items := data.Items
	for _, a := range data.Results {
		items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, ""})
	}
	if len(items) == 0 {
		for _, a := range data.Feed {
			items = append(items, formattedNewsItem{a.Title, a.Description, a.Category, a.URL, a.PostedAt, a.Published})
		}
	}
	if len(items) == 0 {
		return "No news available."
	}

	if data.Query != "" && (data.Freshness.Status == "mostly_stale" || data.Freshness.Status == "stale" || strings.TrimSpace(data.Freshness.Notice) != "") {
		sort.SliceStable(items, func(i, j int) bool {
			left := newsResultItemTime(items[i])
			right := newsResultItemTime(items[j])
			if left.IsZero() || right.IsZero() {
				return false
			}
			return left.After(right)
		})
	}

	if data.Query == "" {
		catOrder := []string{}
		catItems := map[string][]formattedNewsItem{}
		for _, a := range items {
			cat := a.Category
			if cat == "" {
				cat = "_"
			}
			if _, ok := catItems[cat]; !ok {
				catOrder = append(catOrder, cat)
			}
			catItems[cat] = append(catItems[cat], a)
		}
		var mixed []formattedNewsItem
		maxPerCat := 3
		for round := 0; round < maxPerCat; round++ {
			for _, cat := range catOrder {
				if round < len(catItems[cat]) {
					mixed = append(mixed, catItems[cat][round])
				}
			}
		}
		items = mixed
	}

	if len(items) > 20 {
		items = items[:20]
	}
	var sb strings.Builder
	if data.Query != "" {
		sb.WriteString(fmt.Sprintf("News results for %q:\n", data.Query))
	} else {
		sb.WriteString("Latest news:\n")
	}
	if freshnessNotice := strings.TrimSpace(data.Freshness.Notice); freshnessNotice != "" {
		sb.WriteString("Freshness caveat: " + freshnessNotice + "\n")
	} else if data.Freshness.Status == "stale" || data.Freshness.Status == "no_dated_results" {
		requestedDate := strings.TrimSpace(data.Freshness.RequestedDate)
		if requestedDate == "" {
			requestedDate = "the requested date"
		}
		if data.Freshness.Status == "no_dated_results" {
			sb.WriteString("Freshness caveat: No dated news results were available for " + requestedDate + "; do not present these results as today's news without that caveat.\n")
		} else {
			freshest := strings.TrimSpace(data.Freshness.FreshestPostedAt)
			if freshest != "" {
				if t, err := time.Parse(time.RFC3339, freshest); err == nil {
					freshest = t.UTC().Format("2006-01-02")
				}
			}
			if freshest == "" {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; lead with a freshness caveat before older items.\n")
			} else {
				sb.WriteString("Freshness caveat: No same-day news results were available for " + requestedDate + "; the freshest result is from " + freshest + ", so lead with a freshness caveat before older items.\n")
			}
		}
	}
	for i, a := range items {
		line := fmt.Sprintf("%d. %s", i+1, a.Title)
		if desc := cleanNewsDescription(a.Description); desc != "" {
			line += " — " + desc
		}
		if label := conciseNewsSourceDateLabel(a); label != "" {
			line += " (" + label + ")"
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func cleanNewsDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	desc = strings.TrimRight(desc, " \t\n\r")
	for strings.HasSuffix(desc, "...") || strings.HasSuffix(desc, "…") {
		desc = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(desc, "..."), "…"))
	}
	return desc
}

func conciseNewsSourceDateLabel(item formattedNewsItem) string {
	var parts []string
	if source := newsSourceLabel(item.URL); source != "" {
		parts = append(parts, source)
	}
	if date := newsDateLabel(item); date != "" {
		parts = append(parts, date)
	}
	return strings.Join(parts, ", ")
}

func newsSourceLabel(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	return host
}

func newsDateLabel(item formattedNewsItem) string {
	if t := newsResultItemTime(item); !t.IsZero() {
		return t.Format("2 Jan 2006")
	}
	return ""
}

type formattedNewsItem struct {
	Title       string
	Description string
	Category    string
	URL         string
	PostedAt    string
	Published   string
}

func newsResultItemTime(item formattedNewsItem) time.Time {
	for _, value := range []string{item.PostedAt, item.Published} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2 Jan 2006 15:04 MST", "2 Jan 2006", "2006-01-02", "Jan 2, 2006"} {
			if t, err := time.Parse(layout, value); err == nil {
				return t.UTC()
			}
		}
	}
	return time.Time{}
}

func formatVideoResult(result string) string {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Channel string `json:"channel"`
			URL     string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No videos found."
	}
	items := data.Results
	if len(items) > 10 {
		items = items[:10]
	}
	var sb strings.Builder
	sb.WriteString("Video results:\n")
	for i, v := range items {
		line := fmt.Sprintf("%d. %s", i+1, v.Title)
		if v.Channel != "" {
			line += fmt.Sprintf(" (channel: %s)", v.Channel)
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

func formatReminderResult(result string) string {
	var data struct {
		Verse   string `json:"verse"`
		Name    string `json:"name"`
		Hadith  string `json:"hadith"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if data.Verse == "" && data.Hadith == "" && data.Message == "" {
		return "No reflection is available right now."
	}
	var sb strings.Builder
	sb.WriteString("Today's Islamic reflection:\n")
	if data.Verse != "" {
		sb.WriteString(fmt.Sprintf("Verse: %s\n", data.Verse))
	}
	if data.Hadith != "" {
		sb.WriteString(fmt.Sprintf("Saying: %s\n", data.Hadith))
	}
	if data.Name != "" {
		sb.WriteString(fmt.Sprintf("Name: %s\n", data.Name))
	}
	if data.Message != "" {
		sb.WriteString(fmt.Sprintf("Reflection: %s\n", data.Message))
	}
	return sb.String()
}

func formatSearchResult(result string) string {
	var data struct {
		Results []struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Type    string `json:"type"`
		} `json:"results"`
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(result), &data); err == nil && len(data.Results) > 0 {
		var sb strings.Builder
		if data.Query != "" {
			sb.WriteString(fmt.Sprintf("Search results for %q:\n", data.Query))
		} else {
			sb.WriteString("Search results:\n")
		}
		for i, r := range data.Results {
			line := fmt.Sprintf("%d. %s", i+1, r.Title)
			if r.Type != "" {
				line += fmt.Sprintf(" [%s]", r.Type)
			}
			if r.Content != "" {
				snippet := r.Content
				if len(snippet) > 120 {
					snippet = snippet[:120] + "…"
				}
				line += " — " + snippet
			}
			sb.WriteString(line + "\n")
		}
		return sb.String()
	}
	return stripHTMLTags(result)
}

func formatWebFetchResult(result string) string {
	var data struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	var sb strings.Builder
	if data.Title != "" {
		sb.WriteString(fmt.Sprintf("Page: %s\n", data.Title))
	}
	if data.URL != "" {
		sb.WriteString(fmt.Sprintf("URL: %s\n", data.URL))
	}
	sb.WriteString("\n")
	content := data.Content
	if len(content) > 8000 {
		content = content[:8000] + "\n\n[Content truncated for brevity]"
	}
	sb.WriteString(content)
	return sb.String()
}

func stripHTMLTags(s string) string {
	var sb strings.Builder
	inTag := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '<':
			inTag = true
		case s[i] == '>':
			inTag = false
			sb.WriteByte(' ')
		case !inTag:
			sb.WriteByte(s[i])
		}
	}
	out := strings.Join(strings.Fields(sb.String()), " ")
	if len(out) > 2000 {
		out = out[:2000] + "…"
	}
	return out
}

func formatPlacesResult(result string, args map[string]any) string {
	var data struct {
		Results []placeItem `json:"results"`
		Count   int         `json:"count"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}
	if len(data.Results) == 0 {
		return "No places found."
	}

	q := ""
	near := ""
	if args != nil {
		if v, ok := args["q"]; ok {
			q = fmt.Sprintf("%v", v)
		}
		if v, ok := args["near"]; ok {
			near = fmt.Sprintf("%v", v)
		}
		if near == "" {
			if v, ok := args["address"]; ok {
				near = fmt.Sprintf("%v", v)
			}
		}
	}

	var sb strings.Builder
	header := fmt.Sprintf("Found %d place(s)", len(data.Results))
	if q != "" && near != "" {
		header += fmt.Sprintf(" matching %q near %s", q, near)
	} else if q != "" {
		header += fmt.Sprintf(" matching %q", q)
	} else if near != "" {
		header += fmt.Sprintf(" near %s", near)
	}
	sb.WriteString(header + ":\n")
	for i, p := range data.Results {
		line := fmt.Sprintf("%d. %s", i+1, p.Name)
		if p.Category != "" {
			line += " (" + p.Category + ")"
		}
		if p.Address != "" {
			line += " — " + p.Address
		}
		sb.WriteString(line + "\n")
	}
	return sb.String()
}

type placeItem struct {
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Address  string  `json:"address"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

func formatWalletBalanceResult(result string) string {
	var data struct {
		Credits *int   `json:"credits"`
		Balance *int   `json:"balance"`
		Address string `json:"address"`
		USDC    string `json:"usdc"`
		Network string `json:"network"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return result
	}

	var sb strings.Builder
	if credits := data.Credits; credits != nil || data.Balance != nil {
		n := 0
		if credits != nil {
			n = *credits
		} else {
			n = *data.Balance
		}
		fmt.Fprintf(&sb, "Credit balance: %d credits ($%d.%02d). Top up at /account/topup.\n", n, n/100, n%100)
	}
	switch {
	case data.USDC != "":
		net := data.Network
		if net == "" {
			net = "Base"
		}
		fmt.Fprintf(&sb, "Wallet: %s holds $%s USDC on %s.\n", data.Address, data.USDC, net)
	case data.Address != "":
		fmt.Fprintf(&sb, "Wallet address: %s.\n", data.Address)
	}
	if sb.Len() == 0 {
		if data.Text != "" {
			return data.Text
		}
		return result
	}
	return sb.String()
}

func formatAppsSearchResult(result string) string {
	var apps []struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &apps); err != nil {
		return result
	}
	if len(apps) == 0 {
		return "No apps found."
	}
	var sb strings.Builder
	sb.WriteString("Apps:\n")
	for i, a := range apps {
		tagInfo := ""
		if a.Tags != "" {
			tagInfo = " [" + a.Tags + "]"
		}
		sb.WriteString(fmt.Sprintf("%d. %s (%s) — %s%s %d installs /apps/%s\n",
			i+1, a.Name, a.Slug, a.Description, tagInfo, a.Installs, a.Slug))
	}
	return sb.String()
}

func formatAppsReadResult(result string) string {
	var a struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Author      string `json:"author"`
		Tags        string `json:"tags"`
		Installs    int    `json:"installs"`
	}
	if err := json.Unmarshal([]byte(result), &a); err != nil {
		return result
	}
	if a.Name == "" {
		return result
	}
	tagLine := ""
	if a.Tags != "" {
		tagLine = fmt.Sprintf("Tags: %s\n", a.Tags)
	}
	return fmt.Sprintf("App: %s\nID: %s\nDescription: %s\nAuthor: %s\n%sInstalls: %d\nURL: /apps/%s\n",
		a.Name, a.Slug, a.Description, a.Author, tagLine, a.Installs, a.Slug)
}

func formatAppsBuildResult(result string) string {
	var data struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return "App HTML generated successfully."
	}
	if data.HTML == "" {
		return result
	}
	snippet := data.HTML
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	return fmt.Sprintf("Generated app HTML (%d bytes). Preview:\n%s", len(data.HTML), snippet)
}
