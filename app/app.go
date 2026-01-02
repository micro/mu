package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"mu/auth"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Package color mapping
var pkgColors = map[string]string{
	"news":  colorCyan,
	"chat":  colorGreen,
	"video": colorPurple,
	"blog":  colorYellow,
	"app":   colorBlue,
	"mail":  colorRed,
}

// Log prints a formatted log message with a colored package prefix
func Log(pkg string, format string, args ...interface{}) {
	color := pkgColors[pkg]
	if color == "" {
		color = colorWhite
	}
	timestamp := time.Now().Format("15:04:05")
	prefix := fmt.Sprintf("%s[%s %s]%s ", color, timestamp, pkg, colorReset)
	fmt.Printf(prefix+format+"\n", args...)
}

//go:embed html/*
var htmlFiles embed.FS

var Template = `
<html lang="%s">
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
    <link rel="preload" href="/account.png" as="image">
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Nunito+Sans:ital,opsz,wght@0,6..12,200..1000;1,6..12,200..1000&display=swap" rel="stylesheet">
    <link rel="manifest" href="/manifest.webmanifest">
    <link rel="stylesheet" href="/mu.css">
    <script src="/mu.js"></script>
  </head>
  <body%s>
    <div id="head">
      <div id="brand">
        <a href="/">Mu</a>
      </div>
      <div id="account" style="-webkit-tap-highlight-color: transparent;">
        <a id="mail-header" href="/mail" style="display: none; margin-right: 2px; -webkit-tap-highlight-color: transparent;"><img src="/mail.png" width="24" height="24" style="vertical-align: middle;"><span class="label">Inbox</span></a>
        <a id="account-header" href="/account" style="display: none; -webkit-tap-highlight-color: transparent;"><img src="/account.png" width="24" height="24" style="vertical-align: middle;"><span class="label">Account</span></a>
        <a id="login-header" href="/login" style="display: none; -webkit-tap-highlight-color: transparent;"><b>Login</b></a>
      </div>
    </div>
    <div id="container">
      <div id="nav-container">
        <div id="nav">
					<a href="/home"><img src="/home.png" style="margin-bottom: 1px"><span class="label">Home</span></a>
					<a href="/blog"><img src="/post.png"><span class="label">Blog</span></a>
					<a href="/chat"><img src="/chat.png"><span class="label">Chat</span></a>
					<a href="/news"><img src="/news.png"><span class="label">News</span></a>
					<a href="/video"%s><img src="/video.png"><span class="label">Video</span></a>
        </div>
      </div>
      <div id="content">
        <h1 id="page-title">%s</h1>
        %s
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

var LoginTemplate = `<html lang="en">
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
	<form id="login" action="/login%s" method="POST">
	  <h1>Login</h1>
	  %s
	  <input id="id" name="id" placeholder="Username" required>
	  <input id="secret" name="secret" type="password" placeholder="Password" required>
	  <br>
	  <button>Login</button>
	</form>
	<p style="text-align: center; margin-top: 20px;"><a href="/signup">Sign up</a> if you don't have an account</p>
      </div>
    </div>
  </body>
</html>
`

var SignupTemplate = `<html lang="en">
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
	<p style="text-align: center; margin-top: 20px;"><a href="/login">Login</a> if you have an account</p>
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
		// Preserve redirect parameter in form action
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}
		w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, "")))
		return
	}

	if r.Method == "POST" {
		r.ParseForm()

		id := r.Form.Get("id")
		secret := r.Form.Get("secret")

		// Preserve redirect parameter for error messages
		redirectParam := ""
		if redirect := r.URL.Query().Get("redirect"); redirect != "" {
			redirectParam = "?redirect=" + url.QueryEscape(redirect)
		}

		if len(id) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p style="color: red;">Username is required</p>`)))
			return
		}
		if len(secret) == 0 {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p style="color: red;">Password is required</p>`)))
			return
		}

		sess, err := auth.Login(id, secret)
		if err != nil {
			w.Write([]byte(fmt.Sprintf(LoginTemplate, redirectParam, `<p style="color: red;">Invalid username or password</p>`)))
			return
		}

		var secure bool

		if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
			secure = true
		}

		// set a new token
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    sess.Token,
			Path:     "/",
			MaxAge:   2592000,
			Secure:   secure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		// Check for pending membership activation
		if pendingCookie, err := r.Cookie("pending_membership"); err == nil && pendingCookie.Value == "true" {
			// Get account and activate membership
			if acc, err := auth.GetAccount(sess.Account); err == nil {
				acc.Member = true
				auth.UpdateAccount(acc)
			}
			// Clear the pending cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "pending_membership",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
		}

		// Check for redirect parameter, default to home
		redirectTo := r.URL.Query().Get("redirect")
		if redirectTo == "" {
			redirectTo = "/home"
		}
		http.Redirect(w, r, redirectTo, 302)
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
			Name:     "session",
			Value:    sess.Token,
			Path:     "/",
			MaxAge:   2592000,
			Secure:   secure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})

		// Check for pending membership activation
		if pendingCookie, err := r.Cookie("pending_membership"); err == nil && pendingCookie.Value == "true" {
			// Get account and activate membership
			if acc, err := auth.GetAccount(sess.Account); err == nil {
				acc.Member = true
				auth.UpdateAccount(acc)
			}
			// Clear the pending cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "pending_membership",
				Value:    "",
				Path:     "/",
				MaxAge:   -1,
				HttpOnly: true,
			})
		}

		// return to home
		http.Redirect(w, r, "/home", 302)
		return
	}
}

func Account(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// Handle POST to update language
	if r.Method == "POST" {
		r.ParseForm()
		newLang := r.Form.Get("language")
		if _, ok := SupportedLanguages[newLang]; ok {
			acc.Language = newLang
			auth.UpdateAccount(acc)
		}
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}

	// Build wallet section
	walletSection := `<h3>Wallet</h3>
		<p>Credits for pay-as-you-go access beyond your daily free quota.</p>
		<p><a href="/wallet">View Wallet →</a> · <a href="/plans">View Plans →</a></p>`

	// Build membership section
	membershipSection := ""
	membershipURL := os.Getenv("MEMBERSHIP_URL")
	if membershipURL != "" {
		if acc.Member {
			membershipSection = `<h3>Membership</h3>
				<p><strong>✓ You are a member!</strong> Thank you for supporting Mu.</p>
				<p><a href="/membership">View membership details</a></p>`
		} else {
			membershipSection = `<h3>Membership</h3>
				<p>Support Mu and get exclusive benefits.</p>
				<p><a href="/membership">Become a Member →</a></p>`
		}
	}

	// Build language options
	currentLang := acc.Language
	if currentLang == "" {
		currentLang = "en"
	}
	languageOptions := ""
	for code, name := range SupportedLanguages {
		selected := ""
		if code == currentLang {
			selected = " selected"
		}
		languageOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, code, selected, name)
	}

	languageSection := fmt.Sprintf(`<h3>Language</h3>
		<p>Sets the page language to help your browser offer automatic translation.</p>
		<form action="/account" method="POST" style="margin-top: 10px;">
			<select name="language" style="padding: 8px; font-size: 14px;">
				%s
			</select>
			<button type="submit" style="margin-left: 10px;">Save</button>
		</form>`, languageOptions)

	// Admin section
	adminSection := ""
	if acc.Admin {
		adminSection = `<div style="margin-top: 20px;">
			<h3>Admin</h3>
			<p><a href="/admin">User Management →</a></p>
			<p><a href="/admin/moderate">Moderation Queue →</a></p>
		</div>`
	}

	// API Tokens section
	tokensSection := `<div style="margin-top: 20px;">
		<h3>API Tokens</h3>
		<p>Personal Access Tokens for API automation.</p>
		<p><a href="/token">Manage Tokens →</a></p>
	</div>`

	content := fmt.Sprintf(`<div style="max-width: 600px;">
		<h2 style="margin-bottom: 15px;">Profile</h2>
		<p><strong>Username:</strong> %s</p>
		<p><strong>Name:</strong> %s</p>
		<p><strong>Joined:</strong> %s</p>
		<p style="margin-top: 10px;"><a href="/@%s">View Public Profile →</a></p>
		
		<div style="margin-top: 20px;">%s</div>
		
		<div style="margin-top: 20px;">%s</div>
		
		<div style="margin-top: 20px;">%s</div>
		
		<div style="margin-top: 20px;">%s</div>
		
		%s
		
		<hr style="margin: 20px 0;">
		<p><a href="/logout" style="color: #dc3545; font-weight: bold;">Logout</a></p>
		</div>`,
		acc.ID,
		acc.Name,
		acc.Created.Format("January 2, 2006"),
		acc.ID,
		walletSection,
		membershipSection,
		languageSection,
		tokensSection,
		adminSection,
	)

	html := RenderHTML("Account", "Account", content)
	w.Write([]byte(html))
}

func Logout(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		http.Redirect(w, r, "/", 302)
		return
	}

	var secure bool

	if h := r.Header.Get("X-Forwarded-Proto"); h == "https" {
		secure = true
	}
	// delete the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	auth.Logout(sess.Token)
	http.Redirect(w, r, "/", 302)
}

// Session handler
func Session(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		// Return guest session instead of error
		guestSess := map[string]interface{}{
			"type": "guest",
		}
		b, _ := json.Marshal(guestSess)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}

	// Get account to include admin/member status
	acc, err := auth.GetAccount(sess.Account)
	response := map[string]interface{}{
		"id":      sess.ID,
		"type":    sess.Type,
		"account": sess.Account,
		"created": sess.Created,
	}

	if err == nil {
		response["admin"] = acc.Admin
		response["member"] = acc.Member
	}

	b, _ := json.Marshal(response)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// Plans handler - shows available options
func Plans(w http.ResponseWriter, r *http.Request) {
	membershipURL := os.Getenv("MEMBERSHIP_URL")

	// Check if user is logged in and their status
	isMember := false
	isLoggedIn := false
	if sess, err := auth.GetSession(r); err == nil {
		isLoggedIn = true
		if acc, err := auth.GetAccount(sess.Account); err == nil {
			isMember = acc.Member || acc.Admin
		}
	}

	// Build the page
	var content strings.Builder

	content.WriteString(`<div class="plans-page">
<h2>How Mu Works</h2>
<p class="intro">Mu is a utility. Use it for free, pay for what you need, or support us with membership.</p>

<div class="plans-grid">`)

	// Free tier
	content.WriteString(`
<div class="plan-card">
	<h3>Free</h3>
	<div class="plan-price">£0</div>
	<ul>
		<li>10 searches per day</li>
		<li>News, video, and chat</li>
		<li>Resets at midnight UTC</li>
	</ul>`)
	if !isLoggedIn {
		content.WriteString(`<a href="/signup" class="plan-btn">Sign Up</a>`)
	} else {
		content.WriteString(`<span class="plan-current">Your baseline</span>`)
	}
	content.WriteString(`</div>`)

	// Pay-as-you-go
	content.WriteString(`
<div class="plan-card">
	<h3>Pay as you go</h3>
	<div class="plan-price">From £5</div>
	<ul>
		<li>Top up your wallet</li>
		<li>1p per news search</li>
		<li>2p per video search</li>
		<li>3p per chat query</li>
		<li>Credits never expire</li>
	</ul>`)
	if isLoggedIn && !isMember {
		content.WriteString(`<a href="/wallet" class="plan-btn">Top Up</a>`)
	} else if !isLoggedIn {
		content.WriteString(`<a href="/signup" class="plan-btn secondary">Sign up first</a>`)
	} else {
		content.WriteString(`<span class="plan-current">Not needed</span>`)
	}
	content.WriteString(`</div>`)

	// Membership
	content.WriteString(`
<div class="plan-card featured">
	<h3>Member</h3>
	<div class="plan-price">£11<span>/month</span></div>
	<ul>
		<li>Unlimited searches</li>
		<li>Unlimited chat AI</li>
		<li>Access to Mail</li>
		<li>Support Mu's development</li>
		<li>Discord community</li>
	</ul>`)
	if isMember {
		content.WriteString(`<span class="plan-current">✓ You're a member</span>`)
	} else if membershipURL != "" {
		content.WriteString(fmt.Sprintf(`<a href="%s" class="plan-btn primary" target="_blank">Become a Member</a>`, membershipURL))
	} else {
		content.WriteString(`<span class="plan-unavailable">Coming soon</span>`)
	}
	content.WriteString(`</div>`)

	content.WriteString(`</div>`) // end plans-grid

	// FAQ section
	content.WriteString(`
<div class="plans-faq">
	<h3>Questions</h3>
	<details>
		<summary>Why charge for searches?</summary>
		<p>Running AI and API calls costs money. The free tier covers casual use. Credits let you pay only for what you use beyond that.</p>
	</details>
	<details>
		<summary>Do credits expire?</summary>
		<p>No. Once you top up, your credits are yours until you use them.</p>
	</details>
	<details>
		<summary>What does membership include?</summary>
		<p>Unlimited access to all features (no quotas, no credits needed), plus Mail for private messaging, and access to our Discord community.</p>
	</details>
</div>`)

	content.WriteString(`</div>`) // end plans-page

	// CSS
	content.WriteString(`
<style>
.plans-page { max-width: 900px; margin: 0 auto; padding: 20px; }
.plans-page .intro { color: #666; margin-bottom: 30px; }
.plans-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 40px; }
.plan-card { background: #fff; border: 1px solid #e0e0e0; border-radius: 12px; padding: 25px; }
.plan-card.featured { border-color: #667eea; border-width: 2px; }
.plan-card h3 { margin: 0 0 10px 0; font-size: 20px; }
.plan-price { font-size: 32px; font-weight: bold; margin-bottom: 20px; }
.plan-price span { font-size: 16px; font-weight: normal; color: #666; }
.plan-card ul { list-style: none; padding: 0; margin: 0 0 20px 0; }
.plan-card li { padding: 8px 0; border-bottom: 1px solid #f0f0f0; font-size: 14px; }
.plan-card li:last-child { border-bottom: none; }
.plan-btn { display: block; text-align: center; padding: 12px 20px; border-radius: 8px; text-decoration: none; font-weight: 500; }
.plan-btn.primary { background: #667eea; color: white; }
.plan-btn.secondary { background: #f0f0f0; color: #333; }
.plan-btn:not(.primary):not(.secondary) { background: #333; color: white; }
.plan-current { display: block; text-align: center; padding: 12px 20px; color: #666; font-size: 14px; }
.plan-unavailable { display: block; text-align: center; padding: 12px 20px; color: #999; font-size: 14px; }
.plans-faq { margin-top: 40px; }
.plans-faq h3 { margin-bottom: 20px; }
.plans-faq details { margin-bottom: 10px; padding: 15px; background: #f9f9f9; border-radius: 8px; }
.plans-faq summary { cursor: pointer; font-weight: 500; }
.plans-faq p { margin: 10px 0 0 0; color: #666; }
</style>`)

	html := RenderHTMLForRequest("Plans", "Choose how you use Mu", content.String(), r)
	w.Write([]byte(html))
}

// Membership handler
func Membership(w http.ResponseWriter, r *http.Request) {
	membershipURL := os.Getenv("MEMBERSHIP_URL")

	// Check if membership is enabled
	if membershipURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if coming from payment provider by parsing domain from membership URL
	referer := r.Header.Get("Referer")
	fromPaymentProvider := false
	if referer != "" && membershipURL != "" {
		u, err := url.Parse(membershipURL)
		if err == nil && u.Host != "" {
			fromPaymentProvider = strings.Contains(referer, u.Host)
		}
	}

	// Check if user is logged in
	sess, err := auth.GetSession(r)
	if err != nil {
		// Not logged in
		if fromPaymentProvider {
			// Set a cookie to track pending membership activation
			http.SetCookie(w, &http.Cookie{
				Name:     "pending_membership",
				Value:    "true",
				Path:     "/",
				MaxAge:   3600, // 1 hour
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
			content := `<h1>Thank you for becoming a member!</h1>
				<p>Your support helps keep Mu independent and sustainable.</p>
				<p>Please login or signup to activate your membership.</p>
				<p>
					<a href="/login" style="margin-right: 15px;">Login</a>
					<a href="/signup">Signup</a>
				</p>`
			html := RenderHTML("Membership", "Thank you!", content)
			w.Write([]byte(html))
			return
		}
		// Show membership page to non-logged-in users
		paymentSection := ""
		if membershipURL != "" {
			paymentSection = fmt.Sprintf(`<h3>Become a Member</h3>
		<p><a href="%s" target="_blank">Join Link →</a></p>`, membershipURL)
		}

		supportSection := ""
		supportURL := os.Getenv("SUPPORT_URL")
		if supportURL != "" {
			supportSection = fmt.Sprintf(`<h3>Support or Feedback</h3>
		<p>Having issues or want to provide feedback</p>
		<p><a href="%s" target="_blank">Contact us →</a></p>`, supportURL)
		}

		donationSection := ""
		donationURL := os.Getenv("DONATION_URL")
		if donationURL != "" {
			donationSection = `<h3>Donate</h3>
		<p>Make a one-time donation <a href="/donate">Make a donation</a> to support Mu.</p>`
		}

		content := fmt.Sprintf(`<h2>Benefits</h2>
		<ul>
			<li>Access to Mail - private messaging and email</li>
			<li>Unlimited searches - news, video, and chat AI</li>
			<li>Vote on new features and platform direction</li>
			<li>Help keep Mu ad-free and sustainable</li>
			<li>Be part of our Discord community</li>
		</ul>

		%s

		%s

		%s`, paymentSection, supportSection, donationSection)
		html := RenderHTML("Membership", "Support Mu", content)
		w.Write([]byte(html))
		return
	}

	// User is logged in
	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		http.Error(w, "Account not found", http.StatusNotFound)
		return
	}

	// If coming from payment provider, activate membership
	if fromPaymentProvider && !acc.Member {
		acc.Member = true
		auth.UpdateAccount(acc)
	}

	// Show membership page
	membershipStatus := ""
	if acc.Member {
		membershipStatus = `<p><strong>You are a member!</strong> Thank you for supporting Mu.</p>`
	}

	paymentSection := ""
	if !acc.Member && membershipURL != "" {
		paymentSection = fmt.Sprintf(`<h3>Become a Member</h3>
				<p><a href="%s" target="_blank">Join Link →</a></p>`, membershipURL)
	}

	supportSection := ""
	supportURL := os.Getenv("SUPPORT_URL")
	if supportURL != "" {
		supportSection = fmt.Sprintf(`<h3>Support or Feedback</h3>
		<p>Having issues or want to provide feedback</p>
		<p><a href="%s" target="_blank">Contact us →</a></p>`, supportURL)
	}

	donationSection := ""
	donationURL := os.Getenv("DONATION_URL")
	if donationURL != "" {
		donationSection = `<h3>Donate</h3>
		<p>Make a one-time donation <a href="/donate">Make a donation</a> to support Mu.</p>`
	}

	content := fmt.Sprintf(`%s
		<h2>Benefits</h2>
		<ul>
			<li>Access to Mail - private messaging and email</li>
			<li>Unlimited searches - news, video, and chat AI</li>
			<li>Vote on new features and platform direction</li>
			<li>Help keep Mu ad-free and sustainable</li>
			<li>Be part of our Discord community</li>
		</ul>

		%s

		%s

		%s
		
		<p style="margin-top: 30px; color: #666;"><a href="/plans">View all plans →</a></p>`,
		membershipStatus,
		paymentSection,
		supportSection,
		donationSection,
	)

	html := RenderHTML("Membership", "Support Mu", content)
	w.Write([]byte(html))
}

// Donate handler
func Donate(w http.ResponseWriter, r *http.Request) {
	donationURL := os.Getenv("DONATION_URL")

	// Check if donations are enabled
	if donationURL == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check if coming from payment provider
	referer := r.Header.Get("Referer")
	u, err := url.Parse(donationURL)
	fromPaymentProvider := err == nil && u.Host != "" && strings.Contains(referer, u.Host)

	if fromPaymentProvider {
		content := `<h1>Thank you for your donation!</h1>
			<p>Your generous support helps keep Mu independent and sustainable.</p>
			<p>Every contribution makes a difference in building a better internet.</p>
			<p><a href="/">← Return Home</a></p>`
		html := RenderHTML("Donate", "Thank you!", content)
		w.Write([]byte(html))
		return
	}

	// Show donation page
	membershipLink := ""
	membershipURL := os.Getenv("MEMBERSHIP_URL")
	if membershipURL != "" {
		membershipLink = `<hr>
		<p>Looking for recurring support? <a href="/membership">Become a member</a> instead.</p>`
	}

	content := fmt.Sprintf(`<h2>Support Mu</h2>
		<p>Help us build a better internet, free from ads and algorithms.</p>
		<p>Your one-time donation supports the ongoing development and operation of Mu.</p>
		<h3>Why Donate?</h3>
		<ul>
			<li>Keep Mu ad-free and independent</li>
			<li>Support development of new features</li>
			<li>Help maintain server infrastructure</li>
			<li>Enable us to focus on users, not profits</li>
		</ul>
		<p><a href="%s">Make a Donation →</a></p>
		%s`, donationURL, membershipLink)

	html := RenderHTML("Donate", "Support Mu", content)
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

// SupportedLanguages maps language codes to their display names
var SupportedLanguages = map[string]string{
	"en": "English",
	"ar": "العربية",
	"zh": "中文",
}

// GetUserLanguage returns the language preference for the current user, defaults to "en"
func GetUserLanguage(r *http.Request) string {
	sess, err := auth.GetSession(r)
	if err != nil {
		return "en"
	}
	acc, err := auth.GetAccount(sess.Account)
	if err != nil {
		return "en"
	}
	if acc.Language == "" {
		return "en"
	}
	return acc.Language
}

// RenderHTML renders the given html in a template with default language (English)
func RenderHTML(title, desc, html string) string {
	return RenderHTMLWithLang(title, desc, html, "en")
}

// RenderHTMLForRequest renders the given html in a template using the user's language preference
func RenderHTMLForRequest(title, desc, html string, r *http.Request) string {
	lang := GetUserLanguage(r)
	return RenderHTMLWithLang(title, desc, html, lang)
}

// RenderHTMLWithLang renders the given html in a template with specified language
func RenderHTMLWithLang(title, desc, html, lang string) string {
	if lang == "" {
		lang = "en"
	}
	return fmt.Sprintf(Template, lang, title, desc, "", "", title, html)
}

func RenderHTMLWithLogout(title, desc, html string, showLogout bool) string {
	logoutStyle := ""
	if !showLogout {
		logoutStyle = ` style="display: none;"`
	}
	return fmt.Sprintf(Template, "en", title, desc, "", logoutStyle, title, html)
}

// RenderHTMLWithLogoutAndLang renders the given html in a template with logout control and language
func RenderHTMLWithLogoutAndLang(title, desc, html string, showLogout bool, lang string) string {
	if lang == "" {
		lang = "en"
	}
	logoutStyle := ""
	if !showLogout {
		logoutStyle = ` style="display: none;"`
	}
	return fmt.Sprintf(Template, lang, title, desc, "", logoutStyle, title, html)
}

// RenderString renders a markdown string as html
func RenderString(v string) string {
	return string(Render([]byte(v)))
}

// RenderTemplate renders a markdown string in a html template
func RenderTemplate(title string, desc, text string) string {
	return fmt.Sprintf(Template, "en", title, desc, "", "", title, RenderString(text))
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
