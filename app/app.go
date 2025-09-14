package app

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sort"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

//go:embed html/*
var htmlFiles embed.FS

var Template = `
<html>
  <head>
    <title>%s | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="description" content="%s">
    <meta name="referrer" content="no-referrer"/>
    <link rel="preload" href="/home.png" as="image">
    <link rel="preload" href="/chat.png" as="image">
    <link rel="preload" href="/news.png" as="image">
    <link rel="preload" href="/video.png" as="image">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css">
    <script src="/mu.js"></script>
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
      <div id="nav">
      <a href="/home"><img src="home.png" style="margin-bottom: 1px">Home</a>
        <a href="/chat"><img src="chat.png">Chat</a>
        <a href="/news"><img src="news.png">News</a>
        <a href="/video"><img src="video.png">Video</a>
      </div>
    </div>
    <div id="container">
      <div id="content">%s</div>
    </div>
    </div>
  <script>
      if (navigator.serviceWorker) {
        navigator.serviceWorker.register (
          '/mu.js',
          {scope: '/'}
        )
      }
  </script>
  </body>
</html>
`

var CardTemplate = `
<!-- %s -->
<div id="%s" class="card">
  <h4>%s</h4>
  %s
</div>
`

func Link(name, ref string) string {
	return fmt.Sprintf(`<a href="%s" class="link">%s</a>`, ref, name)
}

func Head(app string, refs []string) string {
	sort.Strings(refs)

	var head string

	// create head for channels
	for _, ref := range refs {
		head += fmt.Sprintf(`<a href="/%s#%s" class="head">%s</a>`, app, ref, ref)
	}

	head += `<hr><br>`

	return head
}

func Card(id, title, content string) string {
	return fmt.Sprintf(CardTemplate, id, id, title, content)
}

// Render a markdown document as html
func Render(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}

// RenderHTML renders the given html in a template
func RenderHTML(title, desc, html string) string {
	return fmt.Sprintf(Template, title, desc, html)
}

// RenderString renders a markdown string as html
func RenderString(v string) string {
	return string(Render([]byte(v)))
}

// RenderTemplate renders a markdown string in a html template
func RenderTemplate(title string, desc, text string) string {
	return fmt.Sprintf(Template, title, desc, RenderString(text))
}

func ServeHTML(html string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	})
}

// ServeStatic serves the static content in app/html
func Serve() http.Handler {
	var staticFS = fs.FS(htmlFiles)
	htmlContent, err := fs.Sub(staticFS, "html")
	if err != nil {
		log.Fatal(err)
	}

	return http.FileServer(http.FS(htmlContent))
}
