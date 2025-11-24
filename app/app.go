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
	"strings"
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
    <link rel="preload" href="/post.png" as="image">
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
					<a href="/news"><img src="news.png"><span class="label">News</span></a>
					<a href="/posts"><img src="post.png"><span class="label">Posts</span></a>
					<a href="/video"><img src="video.png"><span class="label">Video</span></a>
					<a id="logout-link" href="/logout"%s><img src="logout.png"><span class="label">Logout</span></a>
        </div>
      </div>
      <div id="content">
        <h1 id="page-title">%s</h1>
        %s
      </div>
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
	  %s
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
	  %s
	  <input id="id" name="id" placeholder="Username (4-24 chars, lowercase)" required>
	  <input id="name" name="name" placeholder="Name (optional)">
  	  <input id="secret" name="secret" type="password" placeholder="Password (min 6 chars)" required>
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

	// create head for topics - plain text format with hash
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
		w.Write([]byte(fmt.Sprintf(LoginTemplate, "")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		secret := r.Form.Get("secret")

		if len(id) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, `<p style="color: red;">Username is required</p>`)))
			return
		}
		if len(secret) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, `<p style="color: red;">Password is required</p>`)))
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, `<p style="color: red;">Invalid username or password</p>`)))
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
		w.Write([]byte(fmt.Sprintf(SignupTemplate, "")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		name := r.Form.Get("name")
		secret := r.Form.Get("secret")

		const usernamePattern = "^[a-z][a-z0-9_]{3,23}$"

		usernameRegex := regexp.MustCompile(usernamePattern)

		if len(id) == 0 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p style="color: red;">Username is required</p>`)))
			return
		}

		if !usernameRegex.MatchString(id) {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p style="color: red;">Invalid username format. Must start with a letter, be 4-24 characters, and contain only lowercase letters, numbers, and underscores</p>`)))
			return
		}

		if len(secret) == 0 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p style="color: red;">Password is required</p>`)))
			return
		}

		if len(secret) < 6 {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p style="color: red;">Password must be at least 6 characters</p>`)))
			return
		}

		// Use username as name if name is not provided
		if len(name) == 0 {
			name = id
		}

		if err := auth.Create(&auth.Account{
			ID:      id,
			Secret:  secret,
			Name:    name,
			Created: time.Now(),
		}); err != nil {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, fmt.Sprintf(`<p style="color: red;">%s</p>`, err.Error()))))
			return
		}

		// login
		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(SignupTemplate, `<p style="color: red;">Account created but login failed. Please try logging in.</p>`)))
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

// Membership handler
func Membership(w http.ResponseWriter, r *http.Request) {
	// Check if coming from GoCardless
	referer := r.Header.Get("Referer")
	fromGoCardless := false
	if referer != "" && (strings.Contains(referer, "gocardless.com") || strings.Contains(referer, "pay.gocardless.com")) {
		fromGoCardless = true
	}

	// Check if user is logged in
	sess, err := auth.GetSession(r)
	if err != nil {
		// Not logged in
		if fromGoCardless {
			content := `<div style="max-width: 600px; margin: 0 auto; padding: 40px 20px; text-align: center;">
				<h1 style="color: #28a745;">🎉 Thank you for becoming a member!</h1>
				<p style="font-size: 1.1em; margin: 20px 0;">Your support helps keep Mu independent and sustainable.</p>
				<p>Please login or signup to activate your membership.</p>
				<div style="margin-top: 30px; display: flex; gap: 15px; justify-content: center;">
					<a href="/login" style="padding: 12px 30px; background: #007bff; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;">Login</a>
					<a href="/signup" style="padding: 12px 30px; background: #28a745; color: white; text-decoration: none; border-radius: 5px; font-weight: bold;">Signup</a>
				</div>
			</div>`
			html := RenderHTML("Membership", "Thank you!", content)
			w.Write([]byte(html))
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// User is logged in
	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// If coming from GoCardless, activate membership
	if fromGoCardless && !acc.Member {
		acc.Member = true
		auth.UpdateAccount(acc)
	}

	// Show membership page
	membershipStatus := ""
	if acc.Member {
		membershipStatus = `<div style="background: #d4edda; border: 1px solid #c3e6cb; color: #155724; padding: 15px; border-radius: 5px; margin-bottom: 20px;">
			<strong>✓ You are a member!</strong> Thank you for supporting Mu.
		</div>`
	}

	content := fmt.Sprintf(`<div style="max-width: 700px; margin: 0 auto;">
		%s
		<h2>Membership Benefits</h2>
		<ul style="font-size: 1.1em; line-height: 1.8;">
			<li>Vote on new features and platform direction</li>
			<li>Exclusive access to latest updates</li>
			<li>Priority support</li>
			<li>Help keep Mu ad-free and sustainable</li>
			<li>Join our Discord community</li>
		</ul>

		%s

		<div style="background: #e7f3ff; padding: 20px; border-radius: 8px; margin-top: 30px; border-left: 4px solid #007bff;">
			<h3 style="margin-top: 0;">💬 Join the Community</h3>
			<p>Connect with other members, share feedback, and participate in discussions:</p>
			<a href="https://discord.gg/jwTYuUVAGh" target="_blank" style="display: inline-block; padding: 10px 20px; background: #5865F2; color: white; text-decoration: none; border-radius: 5px; font-weight: bold; margin-top: 10px;">
				Join Discord →
			</a>
		</div>

		<div style="background: #fff3cd; padding: 20px; border-radius: 8px; margin-top: 20px;">
			<h3 style="margin-top: 0;">Support Through Sadaqah</h3>
			<p>Prefer to make a voluntary donation? We welcome all forms of support to keep Mu independent.</p>
		</div>
	</div>`,
		membershipStatus,
		func() string {
			if !acc.Member {
				return `<div style="margin: 30px 0; padding: 30px; background: #f8f9fa; border-radius: 8px; text-align: center;">
					<h3 style="margin-top: 0;">Become a Member</h3>
					<p style="font-size: 1.2em; font-weight: bold; margin: 20px 0;">£10 / month</p>
					<p style="margin-bottom: 20px;">Secure payment via GoCardless Direct Debit</p>
					<a href="https://pay.gocardless.com/BRT00046P56M824" style="display: inline-block; padding: 15px 40px; background: #007bff; color: white; text-decoration: none; border-radius: 5px; font-weight: bold; font-size: 1.1em;">
						Become a Member →
					</a>
				</div>`
			}
			return ""
		}(),
	)

	html := RenderHTML("Membership", "Support Mu", content)
	w.Write([]byte(html))
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
	return RenderHTMLWithLogout(title, desc, html, true)
}

func RenderHTMLWithLogout(title, desc, html string, showLogout bool) string {
	logoutStyle := ""
	if !showLogout {
		logoutStyle = ` style="display: none;"`
	}
	return fmt.Sprintf(Template, title, desc, logoutStyle, title, html)
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
