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
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/micro/mu/app"
	"github.com/mmcdole/gofeed"
)

//go:embed feeds.json
var f embed.FS

var mutex sync.RWMutex

var feeds = map[string]string{}

var status = map[string]*Feed{}

// cached news html
var html string

// cached headlines
var headlinesHtml string

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
	Title       string
	Description string
	URL         string
	Published   string
	Category    string
	PostedAt    time.Time
	Image       string
}

type Metadata struct {
	Created     int64
	Title       string
	Description string
	Type        string
	Image       string
	Url         string
	Site        string
}

func getPrice(v ...string) map[string]string {
	rsp, err := http.Get(fmt.Sprintf("https://min-api.cryptocompare.com/data/pricemulti?fsyms=%s&tsyms=USD&api_key=%s", strings.Join(v, ","), key))
	if err != nil {
		return nil
	}
	b, _ := ioutil.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	var res map[string]interface{}
	json.Unmarshal(b, &res)
	if res == nil {
		return nil
	}
	prices := map[string]string{}
	for _, t := range v {
		rsp := res[t].(map[string]interface{})
		prices[t] = fmt.Sprintf("%v", rsp["USD"].(float64))
	}
	return prices
}

var tickers = []string{"BTC", "BNB", "ETH"}

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
}

func saveHtml(head, data []byte) {
	if len(data) == 0 {
		return
	}
	mutex.Lock()
	content := fmt.Sprintf("<div>%s</div><div>%s</div>", string(head), string(data))
	html = app.RenderHTML("News", "Read the news", content)
	app.Save("news.html", html)
	mutex.Unlock()
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
		}
	}

	//if len(g.Type) == 0 || len(g.Image) == 0 || len(g.Title) == 0 || len(g.Url) == 0 {
	//	fmt.Println("Not returning", u.String())
	//	return nil
	//}

	return g, nil
}

func parseFeed() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic in feed parser: %v\n", r)
			// You can perform cleanup, logging, or other error handling here.
			// For example, you might send an error to a channel to notify main.
			debug.PrintStack()
		}

		fmt.Println("Relaunching feed parser in 1 minute")
		time.Sleep(time.Minute)

		go parseFeed()
	}()

	p := gofeed.NewParser()

	data := []byte{}
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
			fmt.Printf("Error parsing %s: %v, attempt %d backoff until %v", feed, err, stat.Attempts, stat.Backoff)

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

		head = append(head, []byte(`<a href="#`+name+`" class="head">`+name+`</a>`)...)

		data = append(data, []byte(`<div class=section>`)...)
		data = append(data, []byte(`<hr id="`+name+`" class="anchor">`)...)
		data = append(data, []byte(`<h1>`+name+`</h1>`)...)

		for i, item := range f.Items {
			// only 10 items
			if i >= 10 {
				break
			}

			for _, fn := range replace {
				item.Description = fn(item.Description)
			}

			// get meta
			md, err := getMetadata(item.Link)
			if err != nil {
				fmt.Println("Error parsing", item.Link, err)
				continue
			}

			var val string

			if len(md.Image) > 0 {
				val = fmt.Sprintf(`
	<div class="news">
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover" src="%s">
	    <div class="blurb">
	      <span class="text">%s</span>
	      <span class="description">%s</span>
	    </div>
	  </a>
	</div>
				`, item.Link, md.Image, item.Title, item.Description)
			} else {
				val = fmt.Sprintf(`
	<div class="news">
	  <a href="%s" rel="noopener noreferrer" target="_blank">
	    <img class="cover">
	    <div class="blurb">
	      <span class="text">%s</span>
	      <span class="description">%s</span>
	    </div>
	  </a>
	</div>
				`, item.Link, item.Title, item.Description)
			}
			data = append(data, []byte(val)...)

			post := &Post{
				Title:       item.Title,
				Description: item.Description,
				URL:         item.Link,
				Published:   item.Published,
				PostedAt:    *item.PublishedParsed,
				Category:    name,
			}

			news = append(news, post)

			if i > 0 {
				continue
			}

			// add to headlines / 1 per category
			headlines = append(headlines, post)
		}

		data = append(data, []byte(`</div>`)...)
	}

	head = append(head, []byte(`<hr id="headlines" class="anchor">`)...)

	headline := []byte(`<div class=section>`)

	// get crypto prices
	prices := getPrice(tickers...)

	if prices != nil {
		btc := prices["BTC"]
		eth := prices["ETH"]
		bnb := prices["BNB"]

		var info []byte
		//info = append(info, []byte(`<div id="info"><b>Markets:</b>`)...)
		info = append(info, []byte(`<div id="info">`)...)
		info = append(info, []byte(`<span class="ticker">BTC $`+btc+`</span>`)...)
		info = append(info, []byte(`<span class="ticker">ETH $`+eth+`</span>`)...)
		info = append(info, []byte(`<span class="ticker">BNB $`+bnb+`</span>`)...)
		info = append(info, []byte(`</div>`)...)
		headline = append(headline, info...)
	}

	headline = append(headline, []byte(`<h1>Headlines</h1>`)...)

	// create the headlines
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].PostedAt.After(headlines[j].PostedAt)
	})

	for _, h := range headlines {
		val := fmt.Sprintf(`
			<div class="headline">
			  <a href="#%s" class="category"><small>#%s</small></a>
			  <a href="%s" rel="noopener noreferrer" target="_blank">
			   <span class="text">%s</span>
			   <span class="description">%s</span>
			  </a>
			</div>`,
			h.Category, h.Category, h.URL, h.Title, h.Description)
		headline = append(headline, []byte(val)...)
	}

	headline = append(headline, []byte(`</div>`)...)

	// set the headline
	data = append(headline, data...)

	// save it
	saveHtml(head, data)

	mutex.Lock()
	// set the feed
	feed = news
	// set the headlines
	headlinesHtml = string(headline)
	mutex.Unlock()

	// save the headlines
	app.Save("headlines.html", headlinesHtml)

	// wait 10 minutes
	time.Sleep(time.Minute * 10)

	// go again
	parseFeed()
}

func Load() {
	// load headlines
	b, _ := app.Load("headlines.html")
	headlinesHtml = string(b)

	// load news
	b, _ = app.Load("news.html")
	html = string(b)

	// load the feeds
	loadFeed()

	go parseFeed()
}

func Headlines() string {
	mutex.RLock()
	defer mutex.RUnlock()

	return headlinesHtml
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

	w.Write([]byte(html))
}
