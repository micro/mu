# Apps

## The Idea

Mu is a platform of micro services — small, focused apps that do one thing well. What if users could build and launch their own?

Apps are lightweight, single-purpose web apps hosted on Mu. A currency converter. A pomodoro timer. A habit tracker. A unit converter. A markdown previewer. Small tools that solve real problems without the overhead of a full application.

Think of it as the app store for the small web — no ads, no tracking, no bloat. Just useful tools built by real people.

## Small Web Philosophy

Apps are Mu's answer to the growing "small web" movement — a rejection of bloated, ad-driven platforms in favour of simple, human-authored tools that respect users.

### What is the Small Web?

The small web is a loose collection of projects, standards, and communities that share a common belief: the web should serve people, not platforms. Key players include:

- **Kagi Small Web** — A curated index of 30,000+ non-commercial, human-authored websites. No ads, no AI-generated content, no affiliate links. Content is discovered via RSS feeds and served through Kagi's search engine.
- **IndieWeb** — A community and set of standards (microformats, Webmention, Micropub, IndieAuth) for owning your content on your own domain.
- **Neocities** — Free static hosting in the spirit of GeoCities. A gateway for handcrafted HTML sites.
- **Small Technology Foundation** — Building tools for personal, decentralised web presence with end-to-end encryption.

### How Apps Fit

Apps embody small web values:

| Small Web Principle | How Apps Deliver |
|---|---|
| **Own your tools** | Apps are self-contained HTML — you can copy, fork, and self-host them |
| **No gatekeepers** | No app store review, no super-app runtime, no platform lock-in |
| **Human-authored** | AI assists creation, but humans decide what gets built and published |
| **No tracking** | Apps run in sandboxed iframes with no access to cookies or analytics |
| **Small by design** | Single HTML documents, max 256KB. No build tools, no frameworks required |
| **Discoverable** | Listed in the directory, searchable via MCP, suggested by the agent |

### Contrast with W3C MiniApps

The W3C has a separate [MiniApp specification](https://www.w3.org/2021/miniapps/) designed for super-app ecosystems (WeChat, Alipay, etc.). Those run inside a host app using a dual-thread WebView architecture with proprietary APIs. They require platform gatekeepers.

Mu's apps are the opposite: plain HTML hosted on the open web, accessible via any browser, with no host app required. Where W3C MiniApps centralise, Mu's apps decentralise. Where W3C MiniApps need approval, Mu's are self-published. The only shared concept is "small, focused apps" — the philosophy is fundamentally different.

### Standards We Use

Apps build on established open standards rather than inventing new ones:

- **HTML/CSS/JS** — The app itself. No proprietary markup or APIs.
- **RSS** — Apps directory is discoverable via standard feeds.
- **Web App Manifest** — PWA-compatible for offline installation.
- **postMessage API** — SDK communication between iframe and parent, using the browser's built-in message passing.
- **MCP (Model Context Protocol)** — Agent discovery and creation of apps via `apps_search`, `apps_read`, `apps_create`, and `apps_build` tools.

## How It Works

### What is an App?

An app is a self-contained HTML document (with optional CSS and JavaScript) that runs in a sandboxed iframe on Mu. It has:

- A **name** and **description**
- **HTML content** — the app itself (single document, inline styles/scripts)
- An **author** — the Mu user who created it
- A **category** — what kind of tool it is
- A **slug** — a URL-friendly identifier (`/apps/pomodoro`)

Apps are intentionally simple. No server-side code. No build tools. No frameworks required. Just HTML that does something useful.

### Creating an App

Users create apps through:

1. **The app builder** at `/apps/build` — describe what you want in plain English, the AI generates it, you see a live preview alongside the code, edit either, iterate with follow-on prompts, and save when ready
2. **Templates** — pre-built starters (timer, calculator, tracker, converter, flashcards, notes, AI tool) that match Mu's card styling and can be customised
3. **The web form** at `/apps/new` — paste or write HTML directly
4. **The agent** — ask it to build you an app and it creates one via MCP
5. **The API** — `POST /apps` with JSON

### The App Builder

The builder at `/apps/build` is a split-pane interface:

- **Left pane**: Code editor (syntax-highlighted textarea with tab support)
- **Right pane**: Live preview in a sandboxed iframe
- **Prompt bar**: Describe what you want, AI generates the complete app
- **Follow-on prompts**: Modify the existing app with natural language ("add dark mode", "make the timer play a sound")
- **Templates**: One-click starters that skip boilerplate
- **Save & Launch**: Name it, give it a slug, pick a category, publish

The AI generates apps that conform to Mu's design system — same fonts, spacing, border-radius, and colour palette as the built-in cards. Templates enforce this consistency by providing the right CSS scaffolding.

Example flow:
1. Click "Timer" template — code and preview appear instantly
2. Type "add a lap counter and dark mode toggle" — AI modifies the code
3. Edit the code directly to tweak colours
4. Click "Save & Launch" — app is live at `/apps/my-timer`

### Running an App

Visit `/apps/{slug}` to see the app's page with description, author, and install count. The app itself renders in a sandboxed iframe — isolated from Mu's session, cookies, and DOM.

The iframe sandbox restricts:
- No access to parent page cookies or storage
- No form submission to external URLs
- No popups or navigation of the parent frame
- Scripts are allowed (for interactivity) but contained

### Discovery

Browse all public apps at `/apps`. Filter by category. Search by name. See what's popular. The agent can also suggest apps when relevant — "need a timer? there's an app for that."

### Home Widget

Users can add apps to their home dashboard. The app appears as a card alongside News, Markets, and other built-in features. This uses the existing `Widgets` field on user accounts.

## Data Model

```go
type App struct {
    ID          string    `json:"id"`
    Slug        string    `json:"slug"`
    Name        string    `json:"name"`
    Description string    `json:"description"`
    AuthorID    string    `json:"author_id"`
    Author      string    `json:"author"`
    Icon        string    `json:"icon"`
    HTML        string    `json:"html"`
    Category    string    `json:"category"`
    Public      bool      `json:"public"`
    Installs    int       `json:"installs"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

## Categories

| Category | Examples |
|----------|---------|
| Productivity | Pomodoro timer, to-do list, habit tracker |
| Tools | Calculator, unit converter, colour picker |
| Finance | Currency converter, tip calculator, budget tracker |
| Writing | Markdown previewer, word counter, text formatter |
| Health | BMI calculator, water tracker, breathing exercise |
| Education | Flashcards, quiz maker, times table practice |
| Fun | Random quote, dice roller, decision maker |
| Developer | JSON formatter, regex tester, base64 encoder |

## Security

Apps run in sandboxed iframes with strict CSP:

```
sandbox="allow-scripts"
```

This means:
- **Scripts**: Allowed (apps need interactivity)
- **Same-origin**: Blocked (no access to Mu cookies/storage)
- **Forms**: Blocked (no external form submission)
- **Popups**: Blocked (no window.open)
- **Navigation**: Blocked (can't redirect parent)
- **Downloads**: Blocked

Content is served from a `data:` URI or a separate origin to enforce same-origin isolation.

HTML content is size-limited (256KB max) to prevent abuse.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/apps` | Browse all public apps (HTML + JSON) |
| GET | `/apps/{slug}` | View an app |
| GET | `/apps/{slug}/run` | Run an app (renders iframe) |
| POST | `/apps` | Create a new app |
| PATCH | `/apps/{slug}` | Update your app |
| DELETE | `/apps/{slug}` | Delete your app |

## MCP Tools

Four tools for agent integration:

- **`apps_search`** — Search the apps directory (by name, description, or category)
- **`apps_read`** — Read a specific app's details by slug
- **`apps_create`** — Create a new app (name, description, category, html)
- **`apps_build`** — AI-generate an app from a natural language description

This lets users say "build me a pomodoro timer" and the agent creates a fully functional app. The agent can also search for existing apps, read their details, and suggest relevant tools when users describe a problem.

## SDK

Apps can access Mu platform features through a lightweight JavaScript SDK. The SDK communicates with the parent page via `postMessage` — the parent proxies requests to Mu's backend on the user's behalf.

### Including the SDK

Apps include the SDK by adding a script tag:

```html
<script src="/apps/sdk.js"></script>
```

This injects a global `mu` object with the following methods:

### AI

```javascript
// Ask AI a question
const answer = await mu.ai("What's the capital of France?");

// AI with context
const summary = await mu.ai("Summarise this article", { context: articleText });
```

Calls the existing chat/agent endpoint. Costs credits (same as a chat query).

### Storage

```javascript
// Store data (scoped to this app + user)
await mu.store.set("preferences", { theme: "dark", units: "metric" });

// Retrieve data
const prefs = await mu.store.get("preferences");

// Delete data
await mu.store.del("preferences");

// List keys
const keys = await mu.store.keys();
```

Each app gets a namespaced key-value store (max 100 keys, 64KB per value). Data is persisted server-side and scoped to the app + user combination.

### Fetch

```javascript
// Fetch a URL through Mu's proxy (avoids CORS issues)
const page = await mu.fetch("https://example.com/api/data");
```

Uses the existing web fetch endpoint. Costs credits.

### User

```javascript
// Get current user info (if authenticated)
const user = await mu.user();
// { id: "alice", name: "Alice" }
```

Returns basic user info. No sensitive data (no email, no token).

### How It Works

The SDK uses `window.parent.postMessage` to send requests to the Mu parent page. The parent page listens for these messages, validates the origin, proxies the request to the backend, and posts the result back.

```
App (iframe)              Mu Parent Page            Mu Backend
    |                          |                         |
    |-- postMessage ---------->|                         |
    |   { type: "mu:ai",      |                         |
    |     prompt: "..." }     |-- POST /apps/sdk/ai --->|
    |                          |                         |
    |                          |<-- { result: "..." } ---|
    |<-- postMessage ----------|                         |
    |   { type: "mu:ai:res",  |                         |
    |     result: "..." }     |                         |
```

### Quota

SDK calls consume credits from the user's wallet (not the app author's):

| SDK Method | Credit Cost | Maps To |
|-----------|-------------|---------|
| `mu.ai()` | 1 credit | Chat query |
| `mu.fetch()` | 1 credit | Web fetch |
| `mu.store.*` | Free | App storage |
| `mu.user()` | Free | Session check |

### Security

- SDK requests are authenticated via the parent page's session (the iframe never sees the token)
- Storage is namespaced per app + user — apps cannot read each other's data
- Rate limiting applies per user, same as regular API calls
- The parent validates message origins before processing

## Relation to Marketplace

Apps and marketplace services are complementary:

- **Apps** = client-side HTML tools hosted on Mu (free to create, free to use)
- **Marketplace services** = server-side MCP tools hosted externally (paid per use)

An app could call marketplace services via the agent. A marketplace service could have an app as its UI. But they're independent — apps are simpler, with zero infrastructure cost.

## Why This Works

### For Mu
- **Content**: Users create useful tools that attract other users
- **Engagement**: People return to use their installed apps
- **Differentiation**: No other platform combines AI app generation + hosting + discovery this simply
- **Philosophy**: Perfectly aligned with "apps without ads, algorithms, or tracking"

### For Creators
- **Zero infrastructure**: No hosting, no domain, no deployment pipeline
- **AI-assisted**: Describe what you want, the agent builds it
- **Distribution**: Listed in the directory, discoverable by the agent
- **Simplicity**: Just HTML — the lowest barrier to entry possible

### For Users
- **Useful tools**: Small apps that solve real problems
- **No bloat**: No app store downloads, no sign-ups, no tracking
- **Customisable home**: Add the apps you actually use to your dashboard
- **Trust**: Apps are sandboxed, content is moderated, authors are Mu users

---

## Mu as a Message Stream

Apps are part of a larger vision: Mu as a conversational message delivery system. The home screen presents a unified stream of messages — news updates, market changes, blog posts, social threads, app notifications — all interleaved chronologically. The agent is embedded inline, so you can ask about anything in context.

In this model, an app update ("your timer finished", "new flashcard due") is just another message in the stream. The agent can reference any message, and you can ask follow-up questions about anything you see. Everything is conversational.

---

*This document is a proposal. Share your thoughts on [social](/social).*
