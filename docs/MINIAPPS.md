# Mini Apps

## The Idea

Mu is a platform of micro services — small, focused apps that do one thing well. What if users could build and launch their own?

Mini apps are lightweight, single-purpose web apps hosted on Mu. A currency converter. A pomodoro timer. A habit tracker. A unit converter. A markdown previewer. Small tools that solve real problems without the overhead of a full application.

Think of it as the app store for the small web — no ads, no tracking, no bloat. Just useful tools built by real people.

## How It Works

### What is a Mini App?

A mini app is a self-contained HTML document (with optional CSS and JavaScript) that runs in a sandboxed iframe on Mu. It has:

- A **name** and **description**
- **HTML content** — the app itself (single document, inline styles/scripts)
- An **author** — the Mu user who created it
- A **category** — what kind of tool it is
- A **slug** — a URL-friendly identifier (`/apps/pomodoro`)

Mini apps are intentionally simple. No server-side code. No build tools. No frameworks required. Just HTML that does something useful.

### Creating a Mini App

Users create mini apps through:

1. **The web form** at `/apps/new` — paste or write HTML directly
2. **The agent** — describe what you want and the AI builds it for you
3. **The API** — `POST /apps` with JSON

Example:

```json
{
  "name": "Pomodoro Timer",
  "description": "A simple 25-minute focus timer with break intervals",
  "category": "Productivity",
  "html": "<div id='timer'>25:00</div><button onclick='start()'>Start</button><script>...</script>"
}
```

### Running a Mini App

Visit `/apps/{slug}` to see the app's page with description, author, and install count. The app itself renders in a sandboxed iframe — isolated from Mu's session, cookies, and DOM.

The iframe sandbox restricts:
- No access to parent page cookies or storage
- No form submission to external URLs
- No popups or navigation of the parent frame
- Scripts are allowed (for interactivity) but contained

### Discovery

Browse all public apps at `/apps`. Filter by category. Search by name. See what's popular. The agent can also suggest mini apps when relevant — "need a timer? there's a mini app for that."

### Home Widget

Users can add mini apps to their home dashboard. The app appears as a card alongside News, Markets, and other built-in features. This uses the existing `Widgets` field on user accounts.

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

Mini apps run in sandboxed iframes with strict CSP:

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
| GET | `/apps` | Browse all public mini apps (HTML + JSON) |
| GET | `/apps/{slug}` | View a mini app |
| GET | `/apps/{slug}/run` | Run a mini app (renders iframe) |
| POST | `/apps` | Create a new mini app |
| PATCH | `/apps/{slug}` | Update your mini app |
| DELETE | `/apps/{slug}` | Delete your mini app |

## MCP Tools

Two tools for agent integration:

- **`apps_create`** — Create a new mini app (name, description, category, html)
- **`apps_search`** — Search the mini apps directory

This lets users say "build me a pomodoro timer" and the agent creates a fully functional mini app.

## SDK

Mini apps can access Mu platform features through a lightweight JavaScript SDK. The SDK communicates with the parent page via `postMessage` — the parent proxies requests to Mu's backend on the user's behalf.

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
Mini App (iframe)          Mu Parent Page            Mu Backend
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

Mini apps and marketplace services are complementary:

- **Mini apps** = client-side HTML tools hosted on Mu (free to create, free to use)
- **Marketplace services** = server-side MCP tools hosted externally (paid per use)

A mini app could call marketplace services via the agent. A marketplace service could have a mini app as its UI. But they're independent — mini apps are simpler, with zero infrastructure cost.

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

*This document is a proposal. Share your thoughts on [social](/social).*
