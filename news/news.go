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

	"github.com/PuerkitoBio/goquery"
	"github.com/micro/mu/app"
	"github.com/mmcdole/gofeed"
	"github.com/piquette/finance-go/future"
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

// markets
var marketsHtml string

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
	fmt.Println("Got rates", rates)

	prices := map[string]float64{}

	for k, t := range rates {
		val, err := strconv.ParseFloat(t.(string), 64)
		if err != nil {
			continue
		}
		prices[k] = 1 / val
	}

	// special case for BNB
	rsp, err = http.Get("https://api.coinbase.com/v2/exchange-rates?currency=BNB")
	if err != nil {
		fmt.Println("Error getting prices", err)
		return nil
	}
	b, _ = ioutil.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	json.Unmarshal(b, &res)
	if res == nil {
		return prices
	}

	rates = res["data"].(map[string]interface{})["rates"].(map[string]interface{})
	val, err := strconv.ParseFloat(rates["USD"].(string), 64)
	if err != nil {
		return prices
	}
	prices["BNB"] = val

	// let's get other prices
	for key, ftr := range futures {
		f, err := future.Get(ftr)
		if err != nil {
			fmt.Println("Failed to get future", key, ftr, err)
			continue
		}
		prices[key] = f.Quote.RegularMarketPrice
	}

	return prices
}

var tickers = []string{"GBP", "BNB", "ETH", "BTC", "PAXG"}

var futures = map[string]string{"OIL": "CL=F", "GOLD": "GC=F", "COFFEE": "KC=F", "OATS": "ZO=F", "WHEAT": "KE=F"}

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
	content := fmt.Sprintf("<div>%s</div><div>%s</div>", string(head), string(data))
	html = app.RenderHTML("News", "Read the news", content)
	app.Save("news.html", html)
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

	var data map[string]interface{}

	err = json.Unmarshal(b, &data)
	if err != nil {
		fmt.Println("Error getting reminder", err)
		time.Sleep(time.Minute)

		go getReminder()
		return
	}

	link := fmt.Sprintf("https://reminder.dev%s", data["links"].(map[string]interface{})["verse"].(string))

	html := fmt.Sprintf(`<div class="verse">%s</div>`, data["verse"])
	html += fmt.Sprintf(`<a href="%s"><button>More</button></a>`, link)

	mutex.Lock()
	app.Save("reminder.html", html)
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
	      <span class="title">%s</span>
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
	      <span class="title">%s</span>
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
				Image:       md.Image,
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
	newPrices := getPrices()

	if newPrices != nil {
		info := []byte(`<div id="tickers">`)

		for _, ticker := range tickers {
			price := newPrices[ticker]
			fmt.Println("ticker", ticker, price)
			line := fmt.Sprintf(`<span class="ticker">%s $%.2f</span>`, ticker, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div>`)...)
		marketsHtml = string(info)

		info = []byte(`<div id="futures">`)

		var futureKeys []string
		for ftr, _ := range futures {
			futureKeys = append(futureKeys, ftr)
		}
		sort.Strings(futureKeys)

		for _, ftr := range futureKeys {
			price := newPrices[ftr]
			fmt.Println("future", ftr, price)
			line := fmt.Sprintf(`<span class="ticker">%s $%.2f</span>`, ftr, price)
			info = append(info, []byte(line)...)
		}

		info = append(info, []byte(`</div>`)...)
		marketsHtml += string(info)
	}

	headline = append(headline, []byte(`<h1>Headlines</h1>`)...)

	// create the headlines
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].PostedAt.After(headlines[j].PostedAt)
	})

	for _, h := range headlines {
		val := fmt.Sprintf(`
			<div class="headline">
			<a href="#%s" class="category">%s</a>
			  <a href="%s" rel="noopener noreferrer" target="_blank">
			   <span class="title">%s</span>
			  </a>
			 <span class="description">%s</span>
			</div>`,
			h.Category, h.Category, h.URL, h.Title, h.Description)
		headline = append(headline, []byte(val)...)
	}

	headline = append(headline, []byte(fmt.Sprintf("Updated %s", time.Now().Format(time.RFC850)))...)
	headline = append(headline, []byte(`</div>`)...)

	// set the headline
	data = append(headline, data...)

	mutex.Lock()

	// set the feed
	feed = news
	// set the headlines
	headlinesHtml = string(headline)
	// save it
	saveHtml(head, data)
	// save the headlines
	app.Save("headlines.html", headlinesHtml)
	// save markets
	app.Save("markets.html", marketsHtml)

	mutex.Unlock()

	// wait an hour
	time.Sleep(time.Hour)

	// go again
	go parseFeed()
}

func Load() {
	// load headlines
	b, _ := app.Load("headlines.html")
	headlinesHtml = string(b)

	// save markets
	b, _ = app.Load("markets.html")
	marketsHtml = string(b)

	b, _ = app.Load("reminder.html")

	reminderHtml = string(b)

	// load news
	b, _ = app.Load("news.html")
	html = string(b)

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

	w.Write([]byte(html))
}
