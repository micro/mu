package news

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
	"github.com/mrz1836/go-sanitize"
	"github.com/piquette/finance-go/future"
	nethtml "golang.org/x/net/html"
	"mu/app"
	"mu/data"
)

//go:embed feeds.json
var f embed.FS

var mutex sync.RWMutex

var feeds = map[string]string{}

var status = map[string]*Feed{}

// cached news html
var html string

// cached news body (without full page wrapper)
var newsBodyHtml string

// cached topics html
var topicsHtml string

// cached headlines and content html
var headlinesAndContentHtml string

// cached headlines
var headlinesHtml string

// markets
var marketsHtml string

// cached prices
var cachedPrices map[string]float64

// reminder
var reminderHtml string

// the cached feed
var feed []*Post

// crypto compare api key
var key = os.Getenv("CRYPTO_API_KEY")

type Feed struct {
	Name     string
	URL      string
	Error    error
	Attempts int
	Backoff  time.Time
}

type Post struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Published   string    `json:"published"`
	Category    string    `json:"category"`
	PostedAt    time.Time `json:"posted_at"`
	Image       string    `json:"image"`
	Content     string    `json:"content"`
}

type Metadata struct {
	Created     int64
	Title       string
	Description string
	Type        string
	Image       string
	Url         string
	Site        string
	Content     string
}

// htmlToText converts HTML to plain text with proper spacing
func htmlToText(html string) string {
	if html == "" {
		return ""
	}

	// Parse HTML
	doc, err := nethtml.Parse(strings.NewReader(html))
	if err != nil {
		// If parsing fails, just strip tags the simple way
		re := regexp.MustCompile(`<[^>]*>`)
		text := re.ReplaceAllString(html, " ")
		// Collapse multiple spaces
		re2 := regexp.MustCompile(`\s+`)
		return strings.TrimSpace(re2.ReplaceAllString(text, " "))
	}

	var sb strings.Builder
	var extract func(*nethtml.Node)
	extract = func(n *nethtml.Node) {
		if n.Type == nethtml.TextNode {
			sb.WriteString(n.Data)
		}
		if n.Type == nethtml.ElementNode {
			// Process children first
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extract(c)
			}
			// Add space after block elements
			switch n.Data {
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "a":
				sb.WriteString(" ")
			}
		} else {
			// For non-element nodes, process children
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				extract(c)
			}
		}
	}
	extract(doc)

	// Collapse multiple spaces and trim
	text := sb.String()
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(text, " "))
}

func getDomain(v string) string {
	var host string

	u, err := url.Parse(v)
	if err == nil {
		host = u.Hostname()
	} else {
		parts := strings.Split(v, "/")
		if len(parts) < 3 {
			return v
		}
		host = strings.TrimSpace(parts[2])
	}

	if strings.Contains(host, "github.io") {
		return host
	}

	parts := strings.Split(host, ".")
	if len(parts) == 2 {
		return host
	} else if len(parts) == 3 {
		return strings.Join(parts[1:3], ".")
	}
	return host
}

var Results = `
<div id="topics">%s</div>
<h1 style="margin-top: 0">Results</h1>
<div id="results">
%s
</div>`

func getSummary(post *Post) string {
	return fmt.Sprintf(`Source: <i>%s</i> | %s | <a href="/chat?id=%s>Discuss</a>`, getDomain(post.URL), app.TimeAgo(post.PostedAt), post.ID)
}

func getPrices() map[string]float64 {
	fmt.Println("Getting prices")
	rsp, err := http.Get("https://api.coinbase.com/v2/exchange-rates?currency=USD")
	if err != nil {
		fmt.Println("Error getting prices", err)
		return nil
	}
	b, _ := ioutil.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	var res map[string]interface{}
	json.Unmarshal(b, &res)
	if res == nil {
		return nil
	}

	rates := res["data"].(map[string]interface{})["rates"].(map[string]interface{})

	prices := map[string]float64{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t.(string), 64)
		if err != nil {
			continue
		}
		prices[k] = 1 / val
	}

	// let's get other prices
	for key, ftr := range futures {
		// Use closure to safely handle potential panics from individual futures
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Recovered from panic getting future %s (%s): %v\n", key, ftr, r)
				}
			}()

			f, err := future.Get(ftr)
			if err != nil {
				fmt.Println("Failed to get future", key, ftr, err)
				return
			}
			if f == nil {
				fmt.Println("Future returned nil for", key, ftr)
				return
			}
			// Access the price, which may panic if Quote struct is malformed
			price := f.Quote.RegularMarketPrice
			if price > 0 {
				prices[key] = price
			}
		}()
	}

	return prices
}

var tickers = []string{"GBP", "XLM", "ETH", "BTC", "PAXG"}

var futures = map[string]string{
	"OIL":      "CL=F",
	"GOLD":     "GC=F",
	"COFFEE":   "KC=F",
	"OATS":     "ZO=F",
	"WHEAT":    "KE=F",
	"SILVER":   "SI=F",
	"COPPER":   "HG=F",
	"CORN":     "ZC=F",
	"SOYBEANS": "ZS=F",
}

var futuresKeys = []string{"OIL", "OATS", "COFFEE", "WHEAT", "GOLD"}

var replace = []func(string) string{
	func(v string) string {
		return strings.Replace(v, "© 2025 TechCrunch. All rights reserved. For personal use only.", "", -1)
	},
	func(v string) string {
		return regexp.MustCompile(`<img .*>`).ReplaceAllString(v, "")
	},
	func(v string) string {
		parts := strings.Split(v, "</p>")
		if len(parts) > 0 {
			return strings.Replace(parts[0], "<p>", "", 1)
		}
		return v
	},
	func(v string) string {
		return sanitize.HTML(v)
	},
}

func saveHtml(head, content []byte) {
	if len(content) == 0 {
		return
	}
	body := fmt.Sprintf(`<div id="topics">%s</div><div>%s</div>`, string(head), string(content))
	newsBodyHtml = body
	topicsHtml = string(head)
	headlinesAndContentHtml = string(content)
	html = app.RenderHTML("News", "Read the news", body)
	data.SaveFile("news.html", html)
	data.SaveFile("topics.html", topicsHtml)
	data.SaveFile("headlines_content.html", headlinesAndContentHtml)
}

func loadFeed() {
	// load the feeds file
	data, _ := f.ReadFile("feeds.json")
	// unpack into feeds
	mutex.Lock()
	if err := json.Unmarshal(data, &feeds); err != nil {
		fmt.Println("Error parsing feeds.json", err)
	}
	mutex.Unlock()
}

func backoff(attempts int) time.Duration {
	if attempts > 13 {
		return time.Hour
	}
	return time.Duration(math.Pow(float64(attempts), math.E)) * time.Millisecond * 100
}

func getMetadata(uri string) (*Metadata, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	d, err := goquery.NewDocument(u.String())
	if err != nil {
		return nil, err
	}

	g := &Metadata{
		Created: time.Now().UnixNano(),
	}

	check := func(p []string) bool {
		if p[0] == "twitter" {
			return true
		}
		if p[0] == "og" {
			return true
		}

		return false
	}

	for _, node := range d.Find("meta").Nodes {
		if len(node.Attr) < 2 {
			continue
		}

		p := strings.Split(node.Attr[0].Val, ":")
		if !check(p) {
			p = strings.Split(node.Attr[1].Val, ":")
			if !check(p) {
				continue
			}
			node.Attr = node.Attr[1:]
			if len(node.Attr) < 2 {
				continue
			}
		}

		switch p[1] {
		case "site_name":
			g.Site = node.Attr[1].Val
		case "site":
			if len(g.Site) == 0 {
				g.Site = node.Attr[1].Val
			}
		case "title":
			g.Title = node.Attr[1].Val
		case "description":
			g.Description = node.Attr[1].Val
		case "card", "type":
			g.Type = node.Attr[1].Val
		case "url":
			g.Url = node.Attr[1].Val
		case "image":
			if len(p) > 2 && p[2] == "src" {
				g.Image = node.Attr[1].Val
			} else if len(p) > 2 {
				// skip
				continue
			} else if len(g.Image) == 0 {
				g.Image = node.Attr[1].Val
			}

			// relative url needs fixing
			if len(g.Image) > 0 && g.Image[0] == '/' {
				g.Image = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, g.Image)
			}
		}
	}

	// attempt to get the content
	var fn func(*nethtml.Node)

	fn = func(node *nethtml.Node) {
		if node.Type == nethtml.TextNode {
			first := node.Data[0]
			last := node.Data[len(node.Data)-1]

			data := sanitize.HTML(node.Data)

			if unicode.IsUpper(rune(first)) && last == '.' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else if first == '"' && last == '"' {
				g.Content += fmt.Sprintf(`<p>%s</p>`, data)
			} else {
				g.Content += fmt.Sprintf(` %s`, data)
			}
		}

		if node.FirstChild != nil {
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				fn(c)
			}
		}
	}

	if strings.Contains(u.String(), "cnbc.com") {
		for _, node := range d.Find(".ArticleBody-articleBody").Nodes {
			fn(node)
		}
	}
	//if len(g.Type) == 0 || len(g.Image) == 0 || len(g.Title) == 0 || len(g.Url) == 0 {
	//	fmt.Println("Not returning", u.String())
	//	return nil
	//}

	return g, nil
}

func getReminder() {
	fmt.Println("Getting Reminder at", time.Now().String())
	uri := "https://reminder.dev/api/daily/latest"

	resp, err := http.Get(uri)
	if err != nil {
		fmt.Println("Error getting reminder", err)
		time.Sleep(time.Minute)

		go getReminder()
		return
	}

	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	var val map[string]interface{}

	err = json.Unmarshal(b, &val)
	if err != nil {
		fmt.Println("Error getting reminder", err)
		time.Sleep(time.Minute)

		go getReminder()
		return
	}

	link := fmt.Sprintf("https://reminder.dev%s", val["links"].(map[string]interface{})["verse"].(string))

	html := fmt.Sprintf(`<div class="verse">%s</div>`, val["verse"])
	html += app.Link("More", link)

	mutex.Lock()
	data.SaveFile("reminder.html", html)
	reminderHtml = html
	mutex.Unlock()

	time.Sleep(time.Hour)

	go getReminder()
}

func parseFeed() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic in feed parser: %v\n", r)
			// You can perform cleanup, logging, or other error handling here.
			// For example, you might send an error to a channel to notify main.
			debug.PrintStack()

			fmt.Println("Relaunching feed parser in 1 minute")
			time.Sleep(time.Minute)

			go parseFeed()
		}
	}()

	fmt.Println("Parsing feed at", time.Now().String())
	p := gofeed.NewParser()
	p.UserAgent = "Mu/0.1"

	content := []byte{}
	head := []byte{}
	urls := map[string]string{}
	stats := map[string]Feed{}

	var sorted []string

	mutex.RLock()
	for name, url := range feeds {
		sorted = append(sorted, name)
		urls[name] = url

		if stat, ok := status[name]; ok {
			stats[name] = *stat
		}
	}
	mutex.RUnlock()

	head = []byte(app.Head("news", sorted))

	sort.Strings(sorted)

	// all the news
	var news []*Post
	var headlines []*Post

	for _, name := range sorted {
		feed := urls[name]

		// check last attempt
		stat, ok := stats[name]
		if !ok {
			stat = Feed{
				Name: name,
				URL:  feed,
			}

			mutex.Lock()
			status[name] = &stat
			mutex.Unlock()
		}

		// it's a reattempt, so we need to check what's going on
		if stat.Attempts > 0 {
			// there is still some time on the clock
			if time.Until(stat.Backoff) > time.Duration(0) {
				// skip this iteration
				continue
			}

			// otherwise we've just hit our threshold
			fmt.Println("Reattempting to pull", feed)
		}

		// parse the feed
		f, err := p.ParseURL(feed)
		if err != nil {
			// up the attempts
			stat.Attempts++
			// set the error
			stat.Error = err
			// set the backoff
			stat.Backoff = time.Now().Add(backoff(stat.Attempts))
			// print the error
			fmt.Printf("Error parsing %s: %v, attempt %d backoff until %v\n", feed, err, stat.Attempts, stat.Backoff)

			mutex.Lock()
			status[name] = &stat
			mutex.Unlock()

			// skip ahead
			continue
		}

		mutex.Lock()
		// successful pull
		stat.Attempts = 0
		stat.Backoff = time.Time{}
		stat.Error = nil

		// readd
		status[name] = &stat
		mutex.Unlock()

		content = append(content, []byte(`<div class=section>`)...)
		content = append(content, []byte(`<hr id="`+name+`" class="anchor">`)...)
		content = append(content, []byte(`<h1>`+name+`</h1>`)...)

		for i, item := range f.Items {
			// only 10 items
			if i >= 10 {
				break
			}

			for _, fn := range replace {
				item.Description = fn(item.Description)
			}

			link := item.Link

			fmt.Println("Checking link", link)

			if strings.HasPrefix(link, "https://themwl.org/ar/") {
				link = strings.Replace(link, "themwl.org/ar/", "themwl.org/en/", 1)
				fmt.Println("Replacing mwl ar link", item.Link, link)
			}

			// get meta
			md, err := getMetadata(link)
			if err != nil {
				fmt.Println("Error parsing", link, err)
				continue
			}

			if strings.Contains(link, "themwl.org") {
				item.Title = md.Title
			}

			// extracted content using goquery
			if len(md.Content) > 0 && len(item.Content) == 0 {
				item.Content = md.Content
			}

			// Handle nil PublishedParsed
			var postedAt time.Time
			if item.PublishedParsed != nil {
				postedAt = *item.PublishedParsed
			} else {
				postedAt = time.Now()
			}

			// Clean up description HTML
			cleanDescription := htmlToText(item.Description)

			// create post
			post := &Post{
				ID:          item.GUID,
				Title:       item.Title,
				Description: cleanDescription,
				URL:         link,
				Published:   item.Published,
				PostedAt:    postedAt,
				Category:    name,
				Image:       md.Image,
				Content:     item.Content,
			}

			news = append(news, post)

			// Index the article for search/RAG
			data.Index(
				item.GUID,
				"news",
				item.Title,
				item.Description+" "+item.Content,
				map[string]interface{}{
					"url":       link,
					"category":  name,
					"published": item.Published,
					"image":     md.Image,
				},
			)

			var val string

			if len(md.Image) > 0 {
				val = fmt.Sprintf(`
	<div id="%s" class="news">
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover" src="%s">
	    <div class="blurb">
	      <span class="title">%s</span>
	      <span class="description">%s</span>
	      <span class="text">%s</span>
	    </div>
	  </a>
				`, item.GUID, link, md.Image, item.Title, item.Description, getSummary(post))
			} else {
				val = fmt.Sprintf(`
	<div id="%s" class="news">
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover">
	    <div class="blurb">
	      <span class="title">%s</span>
	      <span class="description">%s</span>
	      <span class="text">%s</span>
	    </div>
	  </a>
				`, item.GUID, link, item.Title, item.Description, getSummary(post))
		}
		

		// close div
		val += `</div>`

		content = append(content, []byte(val)...)

		if i > 0 {
			continue
		}

		// add to headlines / 1 per category
		headlines = append(headlines, post)
		}

		content = append(content, []byte(`</div>`)...)
	}

	headline := []byte(`<div class=section>`)

	// get crypto prices
	newPrices := getPrices()

	if newPrices != nil {
		// Cache the prices for the markets page
		mutex.Lock()
		cachedPrices = newPrices
		mutex.Unlock()

		info := []byte(`<div id="tickers">`)

		for _, ticker := range tickers {
			price := newPrices[ticker]
			line := fmt.Sprintf(`<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div>`)...)
		marketsHtml = string(info)

		info = []byte(`<div id="futures">`)

		for _, ticker := range futuresKeys {
			price := newPrices[ticker]
			line := fmt.Sprintf(`<span class="ticker"><span class="highlight">%s</span>&nbsp;&nbsp;$%.2f</span>`, ticker, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div>`)...)
		marketsHtml += string(info)

		// Index all prices for search/RAG
		for ticker, price := range newPrices {
			data.Index(
				"market_"+ticker,
				"market",
				ticker+" Price",
				fmt.Sprintf("%s is currently trading at $%.2f", ticker, price),
				map[string]interface{}{
					"ticker": ticker,
					"price":  price,
				},
			)
		}
	}

	// create the headlines
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].PostedAt.After(headlines[j].PostedAt)
	})

	for _, h := range headlines {
		val := fmt.Sprintf(`
			<div class="headline">
			<a href="/news#%s" class="category">%s</a>
			  <a href="%s" rel="noopener noreferrer" target="_blank">
			   <span class="title">%s</span>
			  </a>
			 <span class="description">%s</span>
	      		 <span class="text">%s</span>
			`, h.Category, h.Category, h.URL, h.Title, h.Description, getSummary(h))

		// close val
		val += `</div>`
		headline = append(headline, []byte(val)...)
	}

	headline = append(headline, []byte(`</div>`)...)

	// set the headline
	content = append(headline, content...)

	mutex.Lock()

	// set the feed
	feed = news
	// set the headlines
	headlinesHtml = string(headline)
	// save it
	saveHtml(head, content)
	// save the headlines
	data.SaveFile("headlines.html", headlinesHtml)
	// save markets
	data.SaveFile("markets.html", marketsHtml)

	// save the prices as JSON for persistence
	data.SaveJSON("prices.json", cachedPrices)

	mutex.Unlock()

	// wait an hour
	time.Sleep(time.Hour)

	// go again
	go parseFeed()
}

func Load() {
	// load headlines
	b, _ := data.LoadFile("headlines.html")
	headlinesHtml = string(b)

	// load markets
	b, _ = data.LoadFile("markets.html")
	marketsHtml = string(b)

	// load cached prices
	b, _ = data.LoadFile("prices.json")
	if len(b) > 0 {
		var prices map[string]float64
		if err := json.Unmarshal(b, &prices); err == nil {
			mutex.Lock()
			cachedPrices = prices
			mutex.Unlock()
		}
	}

	b, _ = data.LoadFile("reminder.html")

	reminderHtml = string(b)

	// load news
	b, _ = data.LoadFile("news.html")
	html = string(b)

	// load topics
	b, _ = data.LoadFile("topics.html")
	topicsHtml = string(b)

	// load headlines and content
	b, _ = data.LoadFile("headlines_content.html")
	headlinesAndContentHtml = string(b)

	// load the feeds
	loadFeed()

	go parseFeed()

	go getReminder()
}

func Headlines() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return headlinesHtml
}

func Markets() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return marketsHtml
}

func Reminder() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return reminderHtml
}

	func Handler(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	defer mutex.RUnlock()

	if ct := r.Header.Get("Content-Type"); ct == "application/json" {
		resp := map[string]interface{}{
			"feed": feed,
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
		return
	}

	// Build page: topics, then headlines and content
	content := fmt.Sprintf(`
		<div id="topics">%s</div>
		<h2>Headlines</h2>
		<div>%s</div>
	`, topicsHtml, headlinesAndContentHtml)

	html := app.RenderHTMLForRequest("News", "Latest news headlines", content, r)
	w.Write([]byte(html))
}

// GetAllPrices returns all cached prices
func GetAllPrices() map[string]float64 {
	mutex.RLock()
	defer mutex.RUnlock()

	// Return a copy to avoid concurrent map access
	prices := make(map[string]float64)
	if cachedPrices != nil {
		for k, v := range cachedPrices {
			prices[k] = v
		}
	}
	return prices
}

// GetHomepageTickers returns the list of tickers displayed on homepage
func GetHomepageTickers() []string {
	return append([]string{}, tickers...)
}

// GetHomepageFutures returns the list of futures displayed on homepage
func GetHomepageFutures() []string {
	return append([]string{}, futuresKeys...)
}
