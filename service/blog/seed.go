package blog

import (
	"time"

	"mu/internal/app"
)

// buildAppsPostID is a fixed ID so the seeded post is published once and never
// duplicated across restarts.
const buildAppsPostID = "build-your-own-apps"

// ensureSeedPosts publishes the built-in announcement posts if they are missing.
// Idempotent by fixed ID, so it runs on every startup without duplicating, and
// so a new instance gets the post on first load and an existing one gets it on
// the next deploy. Must be called during Load (before postsMap is built).
func ensureSeedPosts() {
	for _, p := range posts {
		if p.ID == buildAppsPostID {
			return
		}
	}
	now := time.Now()
	posts = append(posts, &Post{
		ID:        buildAppsPostID,
		Title:     "Build your own apps on Mu",
		Content:   buildAppsPostContent,
		Author:    "Mu",
		AuthorID:  "mu",
		Tags:      "apps, dev",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err := save(); err != nil {
		app.Log("blog", "seed post save failed: %v", err)
	}
}

const buildAppsPostContent = "Mu is a set of everyday services you own — news, mail, markets, weather, and the rest — with an agent that drives them. But not everything belongs in the core. You shouldn't have to fork Mu to get a bookmarks tool, a reading list, or whatever small thing fits *your* day. That's what apps are for.\n\n" +
	"A Mu app is just an HTML page with a little JavaScript. What makes it more than a static page is the SDK — a global `mu` object that gives the page real backend building blocks, without you standing up a backend.\n\n" +
	"```html\n<script src=\"/apps/sdk.js\"></script>\n```\n\n" +
	"## Who's using it\n\n`mu.user()` tells you who is signed in, so an app can greet them and keep their data separate from everyone else's:\n\n" +
	"```javascript\nconst u = await mu.user();   // { account: 'alice', admin: false }\n```\n\n" +
	"## Storage that knows who you are\n\n`mu.db` is a small database, not just a key-value bucket. Each record has an owner (you) and a public flag, so one app can hold *your* private data and a *shared* public set at the same time:\n\n" +
	"```javascript\nawait mu.db.create('notes', { title: 'Idea', body: '...' });            // private to me\nawait mu.db.create('notes', { title: 'Public tip' }, { public: true }); // shared\n\nawait mu.db.list('notes');                       // my notes\nawait mu.db.list('notes', { scope: 'public' });  // everyone's public notes\nawait mu.db.list('tasks', { where: { done: false, priority: { gte: 2 } } });\n```\n\n" +
	"The owner is set on the server from your session — never from the page — so an app can't read or write someone else's private data even if it tries.\n\n" +
	"## Reach the web, server-side\n\n`mu.web.fetch` makes an HTTP request from the server, so your app gets past CORS and can keep API keys off the client. It's SSRF-guarded — public hosts only:\n\n" +
	"```javascript\nconst res = await mu.web.fetch('https://api.example.com/data');\nconst data = JSON.parse(res.body);\n```\n\n" +
	"## The agent, in your app\n\n`mu.ai(prompt)` for a one-shot answer, `mu.agent(prompt)` for the full tool-using agent — plus typed wrappers for every service (`mu.weather`, `mu.markets`, `mu.news`, and more):\n\n" +
	"```javascript\nconst tldr = await mu.ai('Summarise this', { context: text });\nconst answer = await mu.agent('What changed in the markets today and why?');\n```\n\n" +
	"## A real example: bookmarks\n\nHere's the whole backend of an app that saves links — privately or shared — and fetches each page's title for you. Three primitives, working together:\n\n" +
	"```javascript\nasync function add(url, isPublic) {\n  const res = await mu.web.fetch(url);                                  // read the page\n  const title = (/<title[^>]*>([\\s\\S]*?)<\\/title>/i.exec(res.body) || [])[1] || url;\n  await mu.db.create('bookmarks', { url, title }, { public: isPublic }); // save it\n}\nconst mine   = await mu.db.list('bookmarks', { scope: 'mine' });\nconst shared = await mu.db.list('bookmarks', { scope: 'public' });\n```\n\n" +
	"No server to deploy, no database to provision, no CORS proxy to run. The **Bookmarks** and **Notes** apps ship built-in — open them, or fork them from the editor to make your own.\n\n" +
	"## How to build one\n\n- **Describe it** at `/apps/new` and Mu builds a working app.\n- **Ask the agent** — \"build me a reading list\" — it makes the app and gives you a URL.\n- **Write it yourself** — paste HTML at `/apps/new`, or hand-edit any app at `/apps/{slug}/edit`.\n\n" +
	"Pin the ones you use to the top of your home screen. Keep them private, or make them public for anyone on your instance.\n\n" +
	"It runs on *your* Mu — your instance, your data, your account. Same idea as the rest of Mu: instead of renting a service from a platform that keeps your data, you own the whole stack, and apps are how you extend it.\n\n" +
	"Full SDK reference: **/docs/apps**.\n"
