# Apps

Small, self-contained web apps hosted on Mu. A timer. A calculator. A habit tracker. A unit converter. Just HTML that does something useful — no frameworks, no app store, no tracking.

## What is an App?

An app is a single HTML document (with inline CSS and JavaScript) that runs in a sandboxed iframe. It has a name, description, author, tags, and a URL-friendly slug. Max 256KB.

No server-side code. No build tools. No dependencies. Just HTML.

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
    Tags        string    `json:"tags"`
    Public      bool      `json:"public"`
    Installs    int       `json:"installs"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

## Creating Apps

There are several ways to create an app:

**The builder** at `/apps/build` — a split-pane interface with a code editor on the left and live preview on the right. Describe what you want in the prompt bar and AI generates the complete app. Use follow-on prompts to iterate ("add dark mode", "make the timer play a sound"). Or start from a template and customise.

**Templates** — 8 built-in starters: Blank, Timer, Calculator, Tracker, Converter, Flashcards, Notes, and AI Tool. Each comes with Mu's card styling and is ready to customise.

**The web form** at `/apps/new` — paste or write HTML directly.

**The agent** — ask it to build an app and it creates one via MCP (`apps_build` tool). One step: describe it, get a working app with a URL.

**The API** — `POST /apps/new` with JSON.

## Running Apps

Visit `/apps/{slug}` to see the app page with description and author info. The app renders at `/apps/{slug}/run` inside a sandboxed iframe — isolated from Mu's session, cookies, and DOM.

The iframe sandbox allows scripts (for interactivity) but blocks everything else:

- No access to parent page cookies or storage
- No form submission to external URLs
- No popups or navigation of the parent frame
- No downloads

Content is served with strict CSP headers.

## Code Execution

The agent can run JavaScript in a sandboxed environment using the `apps_run` MCP tool. This is how the agent does calculations, data processing, and computation.

The code runs as a function body — use `return` to output a value. It executes in the same sandboxed iframe environment as apps, with access to SDK features.

```
Agent: "What's 15% tip on £47.50?"
→ apps_run: return (47.50 * 0.15).toFixed(2)
→ Result: "7.13"
```

Scratch code is temporary (auto-cleaned after 1 hour, max 1000 in memory).

## SDK

Apps can access Mu platform features through a lightweight JavaScript SDK. Include it with:

```html
<script src="/apps/sdk.js"></script>
```

This injects a global `mu` object:

### AI

```javascript
const answer = await mu.ai("What's the capital of France?");
const summary = await mu.ai("Summarise this", { context: articleText });
```

### Storage

Persistent key-value store scoped to the app + user (max 100 keys, 64KB per value):

```javascript
await mu.store.set("prefs", { theme: "dark" });
const prefs = await mu.store.get("prefs");
await mu.store.del("prefs");
const keys = await mu.store.keys();
```

### Fetch

Fetch URLs through Mu's proxy (avoids CORS issues):

```javascript
const data = await mu.fetch("https://api.example.com/data");
```

### User

```javascript
const user = await mu.user();
// { id: "alice", name: "Alice" }
```

### Run

Return structured data to the parent page (used by the agent for code execution):

```javascript
mu.run({ bmi: 23.1, category: "normal" });
```

### How It Works

The SDK uses `window.parent.postMessage` to send requests to Mu's parent page. The parent proxies requests to the backend on the user's behalf. The iframe never sees the session token.

```
App (iframe)              Mu Parent Page            Mu Backend
    |                          |                         |
    |-- postMessage ---------->|                         |
    |   { type: "mu:ai" }     |-- POST /apps/.../ai --->|
    |                          |<-- { result: "..." } ---|
    |<-- postMessage ----------|                         |
```

SDK calls consume credits from the user's wallet:

| Method | Cost |
|--------|------|
| `mu.ai()` | 1 credit |
| `mu.fetch()` | 1 credit |
| `mu.store.*` | Free |
| `mu.user()` | Free |

## Discovery

Browse all public apps at `/apps`. Search by name, description, or tag. The agent can suggest apps when relevant.

Apps ship with 6 built-in seed apps: Timer, Calculator, Unit Converter, Flashcards, Notes, and Habit Tracker.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/apps` | Browse all public apps (HTML + JSON) |
| GET | `/apps/{slug}` | View app details |
| GET | `/apps/{slug}/run` | Run app in sandboxed iframe |
| POST | `/apps/new` | Create a new app |
| PATCH | `/apps/{slug}` | Update your app |
| DELETE | `/apps/{slug}` | Delete your app |
| GET | `/apps/build` | Interactive app builder |
| POST | `/apps/build/generate` | AI generation endpoint |
| GET | `/apps/build/templates` | List available templates |
| POST | `/apps/exec` | Create a scratch code execution |
| GET | `/apps/exec?id=xxx` | Run scratch code |
| GET | `/apps/sdk.js` | SDK JavaScript |

## MCP Tools

Five tools for agent integration:

- **`apps_search`** — Search the apps directory by name, description, or tag
- **`apps_read`** — Read a specific app's details by slug
- **`apps_create`** — Create a new app with name, slug, description, tags, and HTML
- **`apps_build`** — AI-generate an app from a natural language description
- **`apps_run`** — Run JavaScript code in a sandbox and return the result

## Security

Apps run in sandboxed iframes:

```
sandbox="allow-scripts"
```

- Scripts allowed for interactivity, but contained
- No access to Mu cookies, storage, or DOM
- No form submission, popups, or navigation of parent
- SDK requests authenticated via parent page session
- Storage namespaced per app + user
- HTML size limited to 256KB
- Rate limiting applies per user
