package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"sort"
	"time"

	"mu/auth"

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
    <link rel="preload" href="/mail.png" as="image">
    <link rel="preload" href="/chat.png" as="image">
    <link rel="preload" href="/blog.png" as="image">
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
      <div id="account">&nbsp;</div>
    </div>
    <div id="container">
      <div id="nav-container">
        <div id="nav">
					<a href="/home"><img src="home.png" style="margin-bottom: 1px"><span class="label">Home</span></a>
					<a href="/chat"><img src="chat.png"><span class="label">Chat</span></a>
					<a href="/blog"><img src="blog.png"><span class="label">Blog</span></a>
					<a href="/news"><img src="news.png"><span class="label">News</span></a>
					<a href="/video"><img src="video.png"><span class="label">Video</span></a>
					<a id="logout-link" href="/logout"><img src="logout.png"><span class="label">Logout</span></a>
        </div>
      </div>
      <div id="topics-container"></div>
      <div id="content">%s</div>
    </div>
    </div>
  <script>
      if (navigator.serviceWorker) {
        navigator.serviceWorker.register (
          '/mu.js',
          {scope: '/'}
        );
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

var LoginTemplate = `<html>
  <head>
    <title>Login | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="referrer" content="no-referrer"/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/mu.css">
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
    </div>
    <div id="container">
      <div id="content">
	<form id="login" action="/login" method="POST">
	  <h1>Login</h1>
	  <input id="id" name="id" placeholder="Username" required>
	  <input id="secret" name="secret" type="password" placeholder="Password" required>
	  <br>
	  <button>Login</button>
	</form>
      </div>
    </div>
  </body>
</html>
`

var SignupTemplate = `<html>
  <head>
    <title>Signup | Mu</title>
    <meta name="viewport" content="width=device-width, initial-scale=1, interactive-widget=resizes-content, viewport-fit=cover" />
    <meta name="referrer" content="no-referrer"/>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/mu.css">
  </head>
  <body>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
    </div>
    <div id="container">
      <div id="content">
	<form id="signup" action="/signup" method="POST">
	  <h1>Signup</h1>
	  <input id="id" name="id" placeholder="Username" required>
	  <input id="name" name="name" placeholder="Name" required>
  	  <input id="secret" name="secret" type="password" placeholder="Password" required>
	  <br>
	  <button>Signup</button>
	</form>
      </div>
    </div>
  </body>
</html>
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

	return head
}

func Card(id, title, content string) string {
	return fmt.Sprintf(CardTemplate, id, id, title, content)
}

// Login handler
func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write([]byte(LoginTemplate))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		secret := r.Form.Get("secret")

		if len(id) == 0 {
			http.Error(w, "missing id", 401)
			return
		}
		if len(secret) == 0 {
			http.Error(w, "missing secret", 401)
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}

		var secure bool

		if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
			secure = true
		}

		// set a new token
		http.SetCookie(w, &http.Cookie{
			Name:   "session",
			Value:  sess.Token,
			Secure: secure,
		})

		// return to home
		http.Redirect(w, r, "/home", 302)
		return
	}
}

// Signup handler
func Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		w.Write([]byte(SignupTemplate))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		name := r.Form.Get("name")
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if !usernameRegex.MatchString(id) {
			http.Error(w, "invalid id", 401)
			return
		}

		if len(secret) < 6 {
			http.Error(w, "invalid secret", 401)
		}

		if len(id) == 0 {
			http.Error(w, "missing id", 401)
			return
		}
		if len(name) == 0 {
			http.Error(w, "missing name", 401)
			return
		}
		if len(secret) == 0 {
			http.Error(w, "missing secret", 401)
			return
		}

		if err := auth.Create(&auth.Account{
			ID:      id,
			Secret:  secret,
			Name:    name,
			Created: time.Now(),
		}); err != nil {
			http.Error(w, err.Error(), 401)
			return
		}

		// login
		sess, err := auth.Login(id, secret)
		if err != nil {
			http.Error(w, err.Error(), 401)
			return
		}

		var secure bool

		if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
			secure = true
		}

		// set a new token
		http.SetCookie(w, &http.Cookie{
			Name:   "session",
			Value:  sess.Token,
			Secure: secure,
		})

		// return to home
		http.Redirect(w, r, "/home", 302)
		return
	}
}

func Logout(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/home", 302)
		return
	}

	var secure bool

	if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
		secure = true
	}
	// set a new token
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Secure: secure,
	})
	auth.Logout(sess.Token)
	http.Redirect(w, r, "/home", 302)
}

// Session handler
func Session(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}

	b, _ := json.Marshal(sess)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
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
