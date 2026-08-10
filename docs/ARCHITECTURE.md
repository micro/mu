# Architecture

Mu is one Go binary. Every capability is a service behind a
[go-micro](https://go-micro.dev) registry, and that registry is the source of
truth — the agent's tools, the sidebar, the app SDK and the status page all
derive from it. Register a service and those surfaces pick it up with no further
wiring.

One surface does not yet derive: the MCP tool list. See
[Deriving MCP tools](#deriving-mcp-tools) at the end.

## Directory layout

```
mu/
├── main.go                 # wiring: Load(), routes, middleware
├── service/                # one directory per service, named for it
│   ├── apps/
│   ├── blog/
│   ├── chat/
│   ├── contacts/
│   ├── db/
│   ├── events/
│   ├── files/
│   ├── images/
│   ├── index/
│   ├── mail/
│   ├── markets/
│   ├── news/
│   ├── places/
│   ├── prayer/
│   ├── search/            # the /search page and its providers, not a service
│   ├── social/
│   ├── stream/
│   ├── tasks/
│   ├── video/
│   ├── wallet/
│   ├── weather/
│   └── web/
├── internal/               # runtime and infrastructure, not features
│   ├── a2a/
│   ├── ai/
│   ├── api/
│   ├── app/
│   ├── auth/
│   ├── blob/
│   ├── cli/
│   ├── data/
│   ├── env/
│   ├── event/
│   ├── flag/
│   ├── imageproxy/
│   ├── cache/
│   ├── origin/
│   ├── safefetch/
│   ├── service/
│   ├── settings/
│   ├── setup/
│   ├── snapshot/
│   ├── usage/
│   ├── user/
│   ├── userdb/
│   └── version/
├── agent/                  # the agent pipeline and micro-agents
├── client/                 # discord, telegram, whatsapp
├── home/                   # landing, home screen, pricing
├── admin/                  # moderation and admin panel
├── scripts/                # deploy, DKIM keys, git hooks, tor
└── docs/                   # this folder, served at /docs
```

`service/search` is the exception to "one directory per service": it holds the
`/search` page and its providers — Brave, the readability reader — while the
capability itself is the `web` service.

`internal/service` is the runtime that hosts services — it is not itself
a service.

## The convention

**service name == directory == route == nav label == tool prefix**

Services live under `service/<name>/`. `internal/service` is the runtime core
that hosts them — it is not itself a service.

The exception is a **headless** service: a capability with no page, so no route
and no nav entry. `index` is headless — it exists for the agent, for apps and
for other services to call.

Two footnotes. `wallet` has a page that predates its service; both are the same
capability with two surfaces. And the `web` service is reached at `/search`,
because "Search" is what a person looks for in the sidebar while `web` is what
the capability is about. The nav label is for humans; the service name is for
callers. `/web` still 301s to `/search` for old links.

**A service is named for a domain, not for an action.** Every one is a noun —
`news`, `mail`, `places`, `web` — and its methods say what to do with that
domain. This is not style: tool names are derived as `service_method`, so a
service named for an action leaves its main method nothing to be called but the
same word. `search` used to be a service, its one method had to be `Search`, and
the derived tool name was `search_search`. Web search is now `web.Search`
alongside `web.Fetch`, matching the `/web/fetch` and `/web/read` routes.
`TestNoMethodRepeatsItsService` holds the line.

Methods that return the current set of something are all called `List` —
`news.List`, `blog.List`, `social.List`, `video.List`, `markets.List`,
`stream.List`, `events.List`, `tasks.List` — so the derived names are uniform and
guessable.

## What is registered

| Service | Page | Agent tool | Account-scoped | What it is |
|---|---|---|---|---|
| `apps` | /apps | ✅ |  | User apps: build, run, edit |
| `blog` | /blog | ✅ |  | Microblogging, daily digests, ActivityPub |
| `chat` | /chat | ✅ |  | Live discussion rooms attached to an item |
| `contacts` | /contacts | ✅ | ✅ | The caller's address book: turn a name into an address |
| `user` | /user | ✅ | ✅ | What an account does about other people's content: save, hide, flag, block. One page listing what you have saved, hidden and blocked, each with an undo |
| `db` | /db | ✅ | ✅ | The caller's own records: named collections that outlive a conversation. Apps keep a separate store each through `mu.db` |
| `events` | /events | ✅ | ✅ | Calendar: scheduling, `.ics` invites, and when you are free — optionally counting an attached Google Calendar |
| `files` | /files | ✅ | ✅ | Per-user file storage: keep a file, get a URL |
| `images` | /images | ✅ | ✅ | Generation, daily image, archive |
| `index` | — | ✅ |  | Search across the caller's own content |
| `mail` | /mail | ✅ | ✅ | SMTP server, inbox, DKIM |
| `markets` | /markets | ✅ |  | Crypto, stocks, futures, commodities, currencies |
| `cache` | — | ✅ | ✅ | A key and a value that survive the conversation. Addressed by label, where db holds collections you query |
| `news` | /news | ✅ |  | RSS aggregation, sentiment, search |
| `places` | /places | ✅ |  | Maps, points of interest, travel time |
| `prayer` | /prayer | ✅ |  | Islamic prayer times, qibla, and a daily reflection |
| `social` | /social | ✅ |  | Threads, replies, status |
| `stream` | /stream | ✅ |  | The console: this instance's own timeline |
| `tasks` | /tasks | ✅ | ✅ | What is to be done, and work handed to the agent |
| `video` | /video | ✅ |  | Curated channels, without ads or recommendations |
| `weather` | /weather | ✅ |  | Forecast and pollen |
| `web` | /search | ✅ |  | Search the web; fetch a URL and return readable content |

## Account-scoped

`Scoped: true` on the Spec, read back through `service.AccountScoped`
(`internal/service/spec.go`). These hold data belonging to one user or spend
their credits, so a caller with no authenticated account cannot reach them
**at all** — the whole service is closed to guests.

That bluntness is why some services that hold per-user data are not scoped.
`stream` is readable by anyone (a guest sees the public timeline) while posting
requires an account, so the check lives in the method rather than on the
service. Marking it scoped would hide the timeline from visitors entirely.
`index` is the same shape: a guest search returns public indexed content and
nothing else, because the caller's own mail is added only when there is a
caller.

Identity comes from the **call context**, never from a request field — see
`internal/service/identity.go`. Handlers read `service.AccountFrom(ctx)`, and no
request struct carries an account. There is nothing to forge: `CallDynamic` and
the agent's `injectAccount` both discard any `account_id` a caller or the model
supplies, so it cannot reach a handler even by accident.

## Destructive methods

**Every registered service becomes an agent tool.** That is the point of
deriving from the registry — register a service and the model can use it.

The guard is per *method*, not per service: `Destructive: true` on the endpoint
in the service's own Spec, read back through `service.Destructive`.

**Withheld from the model, not from the caller.** The check runs in
`blockDestructiveTools` (`agent/native.go`), which wraps the tool loop the model
drives — so the model cannot reach these, and an MCP client holding a token
can. That is the intended boundary: the risk is prompt injection steering the
model, not a person deleting their own file. An unscoped token is the whole
account, deletes included; scope an agent if that is not what you want.

Four are withheld from the model:

- **`wallet.Charge`** — spending should follow from the user's own action
- **`tasks.Delete`** — irreversible, and the user can delete from the page
- **`files.Delete`** — the same, for stored bytes
- **`contacts.Delete`** — the same, for the address book

Everything else on those services is available: the agent can read a balance,
check a cost, create, list, get and update records, store and share a file, and
add and look up a contact.

The reasoning is not that the services are dangerous. The agent reads
attacker-controlled text — an email body, a page it just fetched — so any tool
it holds is a tool prompt injection holds. What earns a place on that list is an
irreversible side effect nobody asked for. Note the agent *already* spends
credits: generating an image costs 15. Withholding a whole service to protect
one method would be both too blunt and inconsistent with that.

A blocked call is refused before it runs and the model is told why, so it can
explain rather than retry.

## Images come from here

Nothing Mu renders points at somebody else's CDN.

A generated image is stored the moment it is made and served from
`/images/file/<id>`; the daily image the same way, from `/images/daily/<date>`.
An article's cover image belongs to the publisher, so it is fetched once,
cached in `internal/blob`, and served from `/img` — `internal/imageproxy`.

The reason is not tidiness. A cross-origin `<img>` hands a third party the
decision about whether the page renders: a hotlink rule, a resource policy, a
blocker's filter list, an expiring signed URL or a rate limit against a page
carrying five hundred of them all end the same way, with a broken image and an
`onerror` that hides it. It is also a request to an ad-tech CDN made on the
reader's behalf, from a product whose pitch is that there is no account in the
way.

`/img` only serves URLs this instance signed, so it is not an open proxy, and it
falls back to redirecting at the original when a fetch fails — some CDNs refuse
a datacentre IP and allow a home one, and the fallback is exactly the old
behaviour, so turning this on can only improve a page, never empty it.

## Adding one

1. Create `service/<name>/`, named for the service — a domain, not an action.
2. Write `Server` with typed methods and `description` tags.
3. Declare `var Spec = service.Spec{…}` with an entry for every method, and
   `service.Register(Spec)` from a `Load()` called in `main.go`.
4. If it holds per-user data or spends credits, set `Scoped: true`.
5. If a method is irreversible and should only follow from a user's own action,
   set `Destructive: true` on it.
6. Add a row above.

Nothing else is needed. The agent, the picker, the app SDK and the status page
all read the registry.

## Deriving MCP tools

MCP tools used to be a second, hand-written registry — `var tools` in
`internal/api/mcp.go` plus `api.RegisterTool` calls in `main.go` — so a newly
registered service became an agent tool, an app SDK call and a nav entry
straight away, but **not** an MCP tool until someone added a stanza.

`api.DeriveTools` runs after the hand-written registrations and adds a
tool for every Spec endpoint that has none — name, description, parameters and
price all from the Spec. A written registration always wins: those carry docs
written for a model, and often return one field of a response rather than the
whole struct.

Price was what kept it open, because a derived tool with no operation would be
an unmetered path to a paid service. An `Endpoint` declares its `Cost`, so a
derived tool is charged exactly like a written one.

Six endpoints had already drifted out of reach this way — `mail_search`,
`places_geocode`, `chat_rooms`, `chat_messages`, `wallet_check`,
`wallet_charge` — and none was withheld on purpose.

## The app SDK

An app is a page plus a JavaScript SDK. It reaches any registered service
through `mu.service(name, method, args)`, so a new service is available to every
app the moment it registers — no SDK change, no wrapper to write.

```javascript
const prices = await mu.service('markets', 'List', { category: 'stocks' });
const all    = await mu.services();  // what this app may call, and each one's methods
```

The call dispatches through the live registry. Account-scoped services need a
signed-in visitor, the account is bound from the session, and any `account_id`
the app sends is discarded — the same identity rules as every other surface.
The typed wrappers below are shortcuts over this, not a separate path.

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

An agent gets the same shape through the `db_*` tools and the `/db` page, in a
separate store: each app has its own namespace, so what an app writes here is
not what `db_create` writes and the other way round. A record published with
`public: true` is the only thing both sides see.

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
- Limits: 2000 records per owner per collection, 64KB per record.

`list` options: `scope` (`mine`|`public`|`all`), `where` (filter on data fields),
`sort` (a data field), `order` (`asc`|`desc`), `limit`.

The same store is reachable outside apps: agents can use the `db_create` / `db_get`
/ `db_list` / `db_delete` tools over MCP and REST (see [MCP docs](MCP.md)). Owner
scoping and the private/public model are identical; an app's data and a user's
API data live in separate namespaces.

`where` matches a scalar for equality, or an operator object per field —
`eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `contains` (substring, or array membership),
`in` (any of a list), `exists` (bool). Multiple operators on a field are ANDed:

```javascript
mu.db.list('tasks', { where: {
  done: false,                     // equality
  priority: { gte: 2 },            // number range
  title: { contains: 'report' },   // substring
  tag: { in: ['work', 'urgent'] }, // membership
} });
```

### Server-side fetch — `mu.web.fetch`

Fetch an external URL from the server, so you avoid CORS and can keep keys off
the client. Returns `{ status, body, headers }`.

```javascript
const res  = await mu.web.fetch('https://api.example.com/data');
const data = JSON.parse(res.body);

// with method / headers / body
await mu.web.fetch(url, { method: 'POST', headers: { Authorization: 'Bearer …' }, body: '…' });
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
mu.weather({ lat, lon });           mu.news();
mu.markets({ category: 'crypto' }); mu.video();
mu.social();                        mu.search('query');
mu.chat('a question');
mu.places.search({ ... });          mu.places.nearby({ ... });
mu.blog.list();  mu.blog.read(id);  mu.blog.create({ ... });
mu.apps.list();  mu.apps.read(slug);
```

## Dependency rules

1. **Subsystems should not import services** — `internal/` is the runtime, not
   the features. The one exception is `wallet`, which `internal/api` and
   `internal/cli` import to price a call; see rule 4
2. **Services import subsystems freely** — that's what they're for
3. **Services should not import each other** — except documented composition layers
4. **`wallet` is the one cross-cutting service** — most services import it for quota
5. **`admin` imports services** — `mail` for the spam filter and blocklists,
   `wallet` for credits and transactions, plus `apps`, `news` and `markets` for
   the panel's own views. An acceptable coupling: admin is a management UI over
   the services, and nothing imports admin

These are conventions, not compiler-enforced boundaries — the import graph is
worth a look before adding an edge to it.
