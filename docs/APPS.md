# Apps

Small, self-contained web apps hosted on Mu — a timer, a notes app, an expense
tracker, a dashboard for some API. Plain HTML, CSS and JavaScript, with a
JavaScript SDK that gives the page real backend building blocks: per-user and
public storage, server-side fetch, the AI and the full agent, and every Mu
service. No frameworks, no build step, no app store, no tracking.

Apps are how you extend Mu without touching the core: build a productivity tool
for yourself, or a universal app others can use — each user's data kept under
their own account, with a public set everyone can see.

## What is an app?

An app is an HTML document (with inline CSS and JavaScript). It has a name,
description, author, tags, and a URL-friendly slug. Max 256KB of HTML.

It renders as a full page at `/apps/{slug}/run` with the SDK injected. SDK calls
go to same-origin, authenticated endpoints (`/apps/{slug}/sdk/...`) — the app
acts as the signed-in user, and the server binds identity server-side, so an app
can never read or write another user's private data.

```go
type App struct {
    ID          string
    Slug        string
    Name        string
    Description string
    AuthorID    string
    Author      string
    Icon        string
    HTML        string   // raw mode: a complete HTML page
    Mode        string   // "" / "raw" = HTML blob, "framework" = blocks + SDK
    Blocks      []Block  // framework mode
    Tags        string
    Price       int      // credits per use (0 = free)
    Public      bool
    Installs    int
    Versions    []Version
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## Creating an app

- **Describe it** at `/apps/new` — type what you want ("an expense tracker", "a
  packing checklist") and Mu builds it.
- **Ask the agent** — "build me a notes app" — it creates one via the
  `apps_create` / `apps_build` MCP tools and gives you a URL.
- **Write it yourself** — `/apps/new` takes raw HTML, and `/apps/{slug}/edit`
  hand-edits any app you own.
- **The API** — `POST /apps/new` (raw HTML) or `POST /apps/generate` (describe).

Include the SDK in your HTML:

```html
<script src="/apps/sdk.js"></script>
```

That injects a global `mu` object.

## The SDK

### Storage — `mu.store` (key/value)

A flat key/value store scoped to this app **and the current user** (max 100 keys,
64KB per value). Good for preferences and small state.

```javascript
await mu.store.set('prefs', { theme: 'dark' });
const prefs = await mu.store.get('prefs');
await mu.store.del('prefs');
const keys = await mu.store.keys();
```

### Database — `mu.db` (collections, private/public)

Named collections of JSON records. Every record has a **server-set owner** (the
signed-in user) and a **public** flag, so one app can hold each user's private
data plus a shared public set. This is the building block for real apps — notes,
lists, posts, trackers — where "mine" and "public" both matter.

```javascript
// Create — private to me, or shared publicly
const note   = await mu.db.create('notes', { title: 'Idea', body: '...' });
const shared = await mu.db.create('notes', { title: 'Public tip' }, { public: true });

// List — scope: 'mine' (default), 'public', or 'all' (mine + public)
const mine   = await mu.db.list('notes');
const public = await mu.db.list('notes', { scope: 'public', sort: 'title', order: 'asc' });
const both   = await mu.db.list('notes', { scope: 'all', where: { done: false }, limit: 50 });

const one = await mu.db.get('notes', id);
await mu.db.update('notes', id, { title: 'Edited' }, { public: false }); // owner only
await mu.db.del('notes', id);                                            // owner only
```

Scoping rules (enforced server-side):

- **owner** is always the authenticated account — never taken from the client.
- **create / update / delete** require a signed-in user and only touch their own
  records (editing someone else's record is refused).
- **list / get** may be used by guests too, but a guest only ever sees `public`
  records; `mine` and `all` need a session.
- Limits: 2000 records per app+collection, 64KB per record.

`list` options: `scope` (`mine`|`public`|`all`), `where` (equality match on data
fields), `sort` (a data field), `order` (`asc`|`desc`), `limit`.

### Server-side fetch — `mu.server.fetch`

Fetch an external URL from the server, so you avoid CORS and can keep keys off
the client. Returns `{ status, body, headers }`.

```javascript
const res  = await mu.server.fetch('https://api.example.com/data');
const data = JSON.parse(res.body);

// with method / headers / body
await mu.server.fetch(url, { method: 'POST', headers: { Authorization: 'Bearer …' }, body: '…' });
```

Guarded against SSRF: `http`/`https` only, and the destination must resolve to a
**public** address — loopback, private ranges, link-local (including the
`169.254.169.254` cloud-metadata endpoint) and multicast are refused, on the
initial URL and every redirect. Responses are capped (2 MiB, 10s). Requires a
signed-in user.

For same-origin Mu endpoints, use `mu.get(path)` / `mu.post(path, body)` instead.

### AI and the agent

```javascript
const answer = await mu.ai('Summarise this', { context: text }); // one-shot
const result = await mu.agent('What changed in the markets today and why?'); // plans, calls tools, synthesises
```

### The user

```javascript
const u = await mu.user();   // { account: 'alice', admin: false, ... } — or { type: 'guest' }
```

### Services

Every Mu service is a typed wrapper:

```javascript
mu.weather({ lat, lon });          mu.news();
mu.markets({ category: 'crypto' }); mu.video();
mu.social();                        mu.search('query');
mu.places.search({ ... });          mu.places.nearby({ ... });
mu.blog.list();  mu.blog.read(id);  mu.blog.create({ ... });
mu.apps.list();  mu.apps.read(slug);
```

### How it works

The app is served as a full page; `mu.*` calls hit same-origin endpoints under
`/apps/{slug}/sdk/...`, authenticated by the session cookie. The server sets the
owner/identity on every write from the session — the app never sees or supplies a
token or account id.

```
App page (/apps/{slug}/run)        Mu backend
    |                                  |
    |-- POST /apps/{slug}/sdk/db ----->|  owner = session account (server-set)
    |<-- { records: [...] } -----------|
```

Credits: `mu.ai` and `mu.agent` consume credits from the user's wallet; `mu.db`,
`mu.store`, `mu.server.fetch` and `mu.user` are included.

## Running and discovery

- `/apps/{slug}` — the app's page (description, author, launch button)
- `/apps/{slug}/run` — the running app
- `/apps` — browse and search public apps
- Pin any app to the top of your home screen from the home settings.

Apps ship with built-in seeds: Hello World, Timer, Calculator, Unit Converter,
Flashcards, Notes (a full private/public example on `mu.db`), and Habit Tracker.

## API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/apps` | Browse public apps (HTML + JSON) |
| GET | `/apps/{slug}` | App details |
| GET | `/apps/{slug}/run` | Run the app |
| GET/POST | `/apps/new` | Create form / create from raw HTML |
| POST | `/apps/generate` | Build from a description |
| GET | `/apps/{slug}/edit` | Edit an app you own |
| PATCH / DELETE | `/apps/{slug}` | Update / delete your app |
| POST | `/apps/{slug}/sdk/db` | `mu.db` — collections data |
| POST | `/apps/{slug}/sdk/store` | `mu.store` — key/value |
| POST | `/apps/{slug}/sdk/fetch` | `mu.server.fetch` — guarded external fetch |
| POST | `/apps/{slug}/sdk/ai` | `mu.ai` |
| GET | `/apps/sdk.js` | The SDK |

## MCP tools

- **`apps_search`** — search the directory
- **`apps_read`** — read an app by slug
- **`apps_create`** — create an app (name, slug, description, tags, HTML)
- **`apps_build`** — build a small app from a description
- **`apps_run`** — run JavaScript in a sandbox and return the result

## Security

- Apps run as the signed-in user; the server binds identity from the session, so
  identity and ownership can never be spoofed by app code.
- `mu.db` and `mu.store` are scoped per app + owner; private records are invisible
  to other users; only owners can modify their records.
- `mu.server.fetch` is SSRF-guarded (public destinations only) and needs a
  session, so an instance can't be used as an open proxy.
- HTML is limited to 256KB; storage and fetch are size/time capped; rate limits
  apply per user.
