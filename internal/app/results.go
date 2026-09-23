package app

import (
	"fmt"
	"html"
	"math"
	"mu/internal/result"
	"net/url"
	"regexp"
	"strings"
)

var cardID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var appID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,49}$`)

var videoID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// VideoPlayer is shared by the standalone watch page and conversation results.
func VideoPlayer(id string, autoplay bool, playerID ...string) string {
	if !videoID.MatchString(id) {
		return `<p class="text-muted">Invalid video ID.</p>`
	}
	u := "https://www.youtube.com/embed/" + id + "?enablejsapi=1&playsinline=1"
	if autoplay {
		u += "&autoplay=1"
	}
	attr := ""
	if len(playerID) > 0 {
		attr = ` id="` + html.EscapeString(playerID[0]) + `"`
	}
	return `<iframe` + attr + ` class="media-player" loading="lazy" width="560" height="315" src="` + u + `" title="YouTube video player" referrerpolicy="strict-origin" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture" playsinline allowfullscreen></iframe>`
}

func Results(items []result.Item) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	var sources []result.Item
	for _, item := range items {
		switch item.Kind {
		case "card":
			if !cardID.MatchString(item.ID) {
				continue
			}
			b.WriteString(`<section class="result-card page-stack"><p class="text-muted">Live service view</p><iframe class="app-widget" loading="lazy" src="/card/` + item.ID + `?embed=1" title="` + html.EscapeString(item.Title) + `"></iframe></section>`)
		case "app":
			if !appID.MatchString(item.ID) {
				continue
			}
			path := "/apps/" + item.ID
			b.WriteString(`<section class="result-card page-stack"><a class="record-title" href="` + path + `">` + html.EscapeString(item.Title) + `</a><iframe class="app-widget" loading="lazy" src="` + path + `?widget=1" title="` + html.EscapeString(item.Title) + `"></iframe></section>`)
		case "note", "doc":
			if item.ID == "" || len(item.ID) > 128 {
				continue
			}
			path := "/notes?id="
			if item.Kind == "doc" {
				path = "/docs?id="
			}
			path += url.QueryEscape(item.ID)
			b.WriteString(`<section class="result-card page-stack"><a class="record-title" href="` + html.EscapeString(path) + `">` + html.EscapeString(item.Title) + `</a><div class="result-preview">` + string(RenderLinesNoImages([]byte(item.Body))) + `</div></section>`)
		case "video":
			if !videoID.MatchString(item.ID) {
				continue
			}
			b.WriteString(`<section class="result-card page-stack">` + VideoPlayer(item.ID, false) + `<a class="record-title" href="https://www.youtube.com/watch?v=` + url.QueryEscape(item.ID) + `">` + html.EscapeString(item.Title) + `</a></section>`)
		case "route":
			b.WriteString(`<section class="result-card page-stack">` + RouteMap(item.Shape) + `<p>` + html.EscapeString(item.Summary) + `</p>`)
			if len(item.Steps) > 0 {
				b.WriteString(`<details><summary>Directions</summary><ol>`)
				for _, step := range item.Steps {
					b.WriteString(`<li>` + html.EscapeString(step) + `</li>`)
				}
				b.WriteString(`</ol></details>`)
			}
			b.WriteString(`</section>`)
		default:
			sources = append(sources, item)
		}
	}

	items = sources
	if len(items) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, `<details class="answer-details"><summary>Sources and results (%d)</summary><div class="answer-details-body">`, len(items))
	for _, item := range items {
		b.WriteString(`<section class="result-card page-stack">`)
		b.WriteString(`<strong>` + html.EscapeString(item.Title) + `</strong>`)
		if item.Summary != "" {
			b.WriteString(`<p>` + html.EscapeString(item.Summary) + `</p>`)
		}
		if len(item.Steps) > 0 {
			b.WriteString(`<details><summary>Directions</summary><ol>`)
			for _, s := range item.Steps {
				b.WriteString(`<li>` + html.EscapeString(s) + `</li>`)
			}
			b.WriteString(`</ol></details>`)
		}
		u, err := url.Parse(item.URL)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			b.WriteString(`<div class="form-actions"><a class="mini-btn" href="` + html.EscapeString(item.URL) + `" target="_blank" rel="noopener noreferrer">Open</a><button class="mini-btn" type="button" data-save-url="` + html.EscapeString(item.URL) + `" data-save-title="` + html.EscapeString(item.Title) + `">Save</button><span role="status"></span></div>`)
		}
		b.WriteString(`</section>`)
	}
	b.WriteString(`</div></details>`)
	return b.String()
}

// RouteMap uses cached same-origin tiles and the route already returned by the tool.
func RouteMap(points []result.Point) string {
	if len(points) < 2 || len(points) > 20000 {
		return ""
	}
	const width, height = 640.0, 320.0
	project := func(p result.Point) (float64, float64) {
		lat := max(-85.0, min(85.0, p.Lat)) * math.Pi / 180
		return (p.Lon + 180) / 360, (1 - math.Log(math.Tan(lat)+1/math.Cos(lat))/math.Pi) / 2
	}
	minX, minY, maxX, maxY := 1.0, 1.0, 0.0, 0.0
	for _, p := range points {
		if math.IsNaN(p.Lat) || math.IsNaN(p.Lon) || math.IsInf(p.Lat, 0) || math.IsInf(p.Lon, 0) || math.Abs(p.Lat) > 90 || math.Abs(p.Lon) > 180 {
			return ""
		}
		x, y := project(p)
		minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
	}
	if minX == maxX && minY == maxY {
		return ""
	}
	z := 1
	for z < 18 {
		size := math.Exp2(float64(z+1)) * 256
		if (maxX-minX)*size > width-48 || (maxY-minY)*size > height-48 {
			break
		}
		z++
	}
	size := math.Exp2(float64(z)) * 256
	left, top := (minX+maxX)*size/2-width/2, (minY+maxY)*size/2-height/2
	var b strings.Builder
	b.WriteString(`<figure class="route-map"><svg viewBox="0 0 640 320" role="img" aria-label="Route map" xmlns="http://www.w3.org/2000/svg">`)
	for x := int(math.Floor(left / 256)); float64(x*256) < left+width; x++ {
		for y := int(math.Floor(top / 256)); float64(y*256) < top+height; y++ {
			if x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
				continue
			}
			fmt.Fprintf(&b, `<image x="%.1f" y="%.1f" width="256" height="256" href="/maps/tiles/world/%d/%d/%d.png"/>`, float64(x*256)-left, float64(y*256)-top, z, x, y)
		}
	}
	var path strings.Builder
	for i, p := range points {
		x, y := project(p)
		op := "L"
		if i == 0 {
			op = "M"
		}
		fmt.Fprintf(&path, "%s%.1f %.1f ", op, x*size-left, y*size-top)
	}
	sx, sy := project(points[0])
	ex, ey := project(points[len(points)-1])
	fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="5" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="%.1f" cy="%.1f" r="5" fill="currentColor"/>`, sx*size-left, sy*size-top, ex*size-left, ey*size-top)
	fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="#1655a0" stroke-width="4" stroke-linejoin="round"/></svg><figcaption>© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a></figcaption></figure>`, path.String())
	return b.String()
}
