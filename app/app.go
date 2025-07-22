package app

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

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
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <style>
	html, body { height: 100%%; width: 100%%; margin: 0; padding: 0; font-family: "Nunito Sans", serif; }
	a { color: #333333; }
	a:visited { color: #333333;}
	#head {
	position: fixed;
	background: white;
	top: 0;
	width: 100%%;
	}
	#head a { margin-right: 10px; color: black; font-weight: bold; text-decoration: none; }
	#container { height: 100%%; max-width: 1024px; margin: 0 auto; padding: 25px;}
	#content { padding-bottom: 100px; }
	#content p { padding: 0 0 25px 0; margin: 0; }
	#desc { margin-bottom: 10px; }
	#title { margin-top: 50px; font-size: 1.2em; font-weight: bold; margin-bottom: 10px; }
	li { margin-bottom: 5px; }
	code {
	  background: whitesmoke;
	  padding: 5px;
	  border-radius: 5px;
	}
	.right {
	  text-align: right;
	}
	.block {
	    text-decoration: none;
	    margin-bottom: 10px;
	    border: 1px solid grey;
	    border-radius: 5px;
	    padding: 10px;
	    display: block;
	}
	@media only screen and (max-width: 600px) {
	  #head a { margin-right: 5px; }
	}
	#brand {
	  display: inline-block;
	  padding: 20px;
	}
	#nav {
	 float: right;
	 padding: 20px 20px 20px 0;
	}
    </style>
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
      <div id="nav">
        <a href="/">Home</a>
      </div>
    </div>
    <div id="container">
      <div id="title">%s</div>
      <div id="desc">%s</div>
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
	return fmt.Sprintf(Template, title, title, desc, html)
}

// RenderString renders a markdown string as html
func RenderString(v string) string {
	return string(Render([]byte(v)))
}

// RenderTemplate renders a markdown string in a html template
func RenderTemplate(title string, desc, text string) string {
	return fmt.Sprintf(Template, title, title, desc, RenderString(text))
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
