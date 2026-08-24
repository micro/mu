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
│     apps  blog  chat  contacts  docs  email  events  files  flights  food
│     hazards  images  mail  markets  news  notes  places  prayer  routes
│     sms  social  stream  tasks  text  transit  video  wallet  weather
│     web  whatsapp
├── internal/               # runtime and infrastructure, not features
│     ai  api  app  auth  backup  blob  cli  contacts  data  env  event  flag
│     geo  google  gtfs  imageproxy  linkmeta  notes  origin  phone  quota
│     safefetch  safety  server  service  settings  setup  snapshot  thread
│     twilio  usage  user  userdb  version  x402
├── agent/                  # the agent pipeline and the agent registry
│   ├── blog/               #   writes the daily opinion
│   ├── digest/             #   writes the daily digest
│   ├── local/              #   models running on this machine
│   ├── micro/              #   the Agent type, the registry, addressing by name
│   └── social/             #   surfaces breaking stories
├── account/                # sign-in, tokens, the credit ledger
├── home/                   # landing, home screen, pricing
├── tool/                   # service.Spec → api.Tool
├── admin/                  # moderation and admin panel
├── scripts/                # deploy, DKIM keys, git hooks, tor
└── docs/                   # this folder, served at /help
```

`internal/thread` is the system of record — what was said, on which
conversation, from which client — and it is written on every turn whether or
not anybody asks. That is why it is substrate rather than a service: a service
is something a caller may choose to use, and an agent that forgot to call this
one would simply stop remembering. The read side is `/inbox`, whose rail lists
every conversation, and `service/recall` for going looking on purpose.

There is no exception to "one directory per service". There was one —
`service/search` held the `/search` page and its providers while the capability
itself was the `web` service — and it cost twice: a directory under `service/`
that was not a service, and a sideways import from `web` to reach its own
provider. Both went away when the two halves became one directory.

`internal/service` is the runtime that hosts services — it is not itself
a service.

## The convention

**service name == directory == route == nav label == tool prefix**

Services live under `service/<name>/`. `internal/service` is the runtime core
that hosts them — it is not itself a service.

There is no exception. `Spec.Headless()` exists — a service with an empty
`Page` — and nothing is, which is worth saying out loud because the escape hatch
being there invites use: `contacts` was documented as headless for a long time
while sitting at `/contacts`, and `recall` shipped without a page on the
reasoning that another page already showed its data.

A service a person cannot find in the sidebar is a capability that exists only
for the agent, and the pitch is that both doors reach the same set of things. If
something seems to want to be a service with no page, the question to ask is
whether it is a service — the last time the answer was dressed up instead, it
was a flag called `Staple` meaning "is a service, but hide it", and deleting the
flag was the fix.

One footnote. The `web` service is reached at `/search`,
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
| `shell` | /shell | ✅ | ✅ | A machine of your own: a container with a shell in it and a `/work` volume that outlives it. `shell_run` takes a shell command and returns its output and exit status; `shell_write`, `shell_read` and `shell_list` move files without going through a shell, which is what makes writing source reliable. Needs Docker — not built into the binary, so an instance without it says so and serves nothing, the same as `browser` without Chromium. One container and one volume per account, capped by `SHELL_MEMORY`, `SHELL_CPUS` and `SHELL_PIDS` — memory defaults to a quarter of the host rather than a flat number, because a flat 2g on a 2GB VM is the whole box and the host OOM killer would pick the Mu server. At most `SHELL_MAX_MACHINES` run at once, and starting one past the cap stops the idlest rather than refusing the caller. `SHELL_SHARED` swaps one container per account for a pool of them over one volume, which is what fits on a small box: `/work/<account>` per caller at `0700`, `/work` sticky like `/tmp`, and every command run as that account's own Unix user — so nobody can read, list or delete anybody else's files, while the machine itself (processes, cgroup, network) is shared. There is no chown anywhere in it, because `--cap-drop ALL` takes `CAP_CHOWN` from root; each account makes its own directory as itself instead. No capabilities, no swap and no route back to the host; `internal/container` builds the argv in one place so that "it never mounts the docker socket" is checkable by reading one function. Idle machines are stopped after `SHELL_IDLE_MINUTES` and their files kept. Priced at 2 credits a command, because a command is CPU here |
| `browser` | /browser | ✅ |  | A real browser over chromedp: read a page after its JavaScript has run, or photograph it. Needs Chromium, which cannot be built into the binary — it looks on the PATH first, so an installed browser needs no configuration; `CHROME_PATH` names a particular one and `BROWSER_URL` points at a DevTools endpoint elsewhere. With none of the three it says so and serves nothing. Priced, because it runs a browser: 3 credits to read, 4 to photograph. Every URL goes through `internal/hosts` first, because an agent pointing it at 127.0.0.1 or the cloud metadata endpoint is the risk |
| `chat` | /chat | ✅ |  | Live discussion rooms attached to an item |
| `contacts` | /contacts | ✅ | ✅ | The caller's address book: turn a name into an address |
| `docs` | /docs | ✅ | ✅ | The caller's own documents: named collections that outlive a conversation. Apps keep a separate store each through `mu.db` |
| `events` | /events | ✅ | ✅ | Calendar: scheduling, `.ics` invites, and when you are free — optionally counting an attached Google Calendar |
| `files` | /files | ✅ | ✅ | Per-user file storage: keep a file, get a URL |
| `images` | /images | ✅ | ✅ | Generation, daily image, archive |
| `mail` | /mail | ✅ | ✅ | Private messages, and an inbox each agent can be reached at. A handle always; an email address when the instance has a mail domain. Everything leaving goes through one path in `outbound.go`, where the price and the gate are applied once — mail out as yourself needs an account this instance can hold to something (`auth.Trusted`: admin, approved, verified, or funded), and answering somebody who wrote to you first is never gated |
| `markets` | /markets | ✅ |  | Crypto, stocks, futures, commodities, currencies. Prices from Coinbase, CoinGecko and Yahoo; conversion from ECB reference rates, keyless, back to 1999 |
| `notes` | /notes | ✅ | ✅ | A title and what is under it, kept between conversations. Addressed by title, where docs holds collections you query |
| `news` | /news | ✅ |  | RSS aggregation, sentiment, search |
| `flights` | /flights | ✅ |  | Where aircraft are, live from ADS-B. No schedule behind it: it reports positions aeroplanes broadcast, never a departure time |
| `places` | /places | ✅ |  | Points of interest, geocoding both ways, elevation — what is there |
| `routes` | /routes | ✅ |  | How to get between two places: time with traffic, turn-by-turn, and which of several is nearest. Split from `places`, which is the Places API's domain where this is the Routes API's |
| `prayer` | /prayer | ✅ |  | Islamic prayer times, qibla, and a daily reflection |
| `archive` | /archive | ✅ |  | Everything this instance has collected, searchable as one thing. Six services write to `internal/data`'s index — news, video, markets, blog, prayer, social — and every reader over it was filtered to one type, so the archive was large and could not be asked a question that crossed a service. A reader and not a store: nothing here archives anything, and deleting it stops nothing being archived. Public only — an entry with an owner is somebody's private record and is never returned, which is what makes this the other archive to `recall` |
| `recall` | /recall | ✅ | ✅ | The caller's own past: search what was said on any client, and read a conversation back. The read over `internal/thread`, which every client writes to on every turn and which is deliberately not a service — a record is not a choice, going looking in it is. Its page is a search box: `/inbox` browses your conversations, this searches every message in all of them. Delete this service and nothing breaks: clients still record, the agent still gets its history, the page still renders |
| `sms` | /sms | ✅ | ✅ | A phone number: text somebody, read what they text back. Twilio. Account-only even when paid: what an anonymous sender spends is the number's reputation |
| `social` | /social | ✅ |  | Threads, replies, status |
| `stream` | /stream | ✅ |  | What has been happening here, announced by the services it happened in |
| `tasks` | /tasks | ✅ | ✅ | What is to be done, and work handed to the agent |
| `text` | /text | ✅ |  | Language work at a fixed price per call: summarise, extract JSON to a schema, classify, translate. Capped at 30,000 characters, because our cost varies with length and the price does not |
| `food` | /food | ✅ |  | Ingredients, allergens and nutrition by barcode from Open Food Facts; UK hygiene ratings from the FSA. Both keyless and both authoritative rather than plausible |
| `hazards` | /services/hazards | ✅ |  | What is going wrong physically: earthquakes live from the USGS, disasters from GDACS. No key, and authoritative rather than plausible — the point of it is that a model would otherwise guess. Flood warnings in force in England come from the Environment Agency, keyless like the rest — the one forecast among them |
| `transit` | /transit | ✅ |  | Public transport: stops near you, what is due, which lines are down. London live from TfL; everywhere else from published GTFS timetables via `internal/gtfs`. No key either way — it works on a fresh install, which is the point. Live outside London comes from two more, both free to register for: `BODS_API_KEY` for where the buses are, `LDBWS_TOKEN` for the board at a station |
| `maps` | /maps | ✅ |  | A map of Britain you can move around, over Ordnance Survey raster tiles. Free. Fetched once and served from `internal/blob` forever after, so a region costs this instance one look however many people use it — what bounds it is `TILE_FETCH_PER_HOUR` cold fetches per account, not a price. `OS_MAPS_KEY` to fetch; without one it still serves what it holds. The tile URL keeps the old word — `/maps/tiles/<style>/<z>/<x>/<y>.png` — because it is pasted into map configs this instance cannot see |
| `video` | /video | ✅ |  | Curated channels, without ads or recommendations |
| `wallet` | /wallet | ✅ | ✅ | A key of your own on Base: an address that holds USDC, and paying an x402-priced tool on another server with it |
| `weather` | /services/weather | ✅ |  | Forecast and pollen through Google, keyed. Air quality, sea state and the historical record through Open-Meteo, keyless — the part that still works on a clone |
| `web` | /web | ✅ |  | Search the web; fetch a URL and return readable content |

## Account-scoped

`Scoped: true` on the Spec, read back through `service.AccountScoped`
(`internal/service/spec.go`). These hold data belonging to one user or spend
their credits, so a caller with no authenticated account cannot reach them
**at all** — the whole service is closed to guests.

That bluntness is why some services that hold per-user data are not scoped.
`stream` is readable by anyone (a guest sees the public timeline) while posting
requires an account, so the check lives in the method rather than on the
service. Marking it scoped would hide the timeline from visitors entirely.
`sms` is the reverse shape: every method is account-only even though sending is
priced, because what an anonymous sender spends is the number's reputation and
that belongs to everybody on the instance.

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

**`Destructive` is not "this method writes".** That is `Writes`, and they were
one flag until a note could be written by a GET. Destructive asks whether the
*model* may hold the tool; Writes asks whether a *GET* may perform it.
`notes.Add` writes and is perfectly safe to hand an agent, so it is `Writes`
and not `Destructive` — and before the split it was neither, which meant
`/api/v1/notes/add?title=x&text=y` was a URL that changed your data when
anything followed it. Anything destructive writes, so ask `service.Changes`
rather than either field. `TestAMethodNamedForAMutationSaysItWrites` holds the
rule, off the naming convention: a method called Add, Create, Send or Pay
changes something by construction.

**Withheld from the model, not from the caller.** The check runs in
`blockDestructiveTools` (`agent/native.go`), which wraps the tool loop the model
drives — so the model cannot reach these, and an MCP client holding a token
can. That is the intended boundary: the risk is prompt injection steering the
model, not a person deleting their own file. An unscoped token is the whole
account, deletes included; scope an agent if that is not what you want.

Some are withheld from the model, by `Destructive: true` on the endpoint —
`tasks.Delete`, `files.Delete`, `contacts.Delete`, `events.Delete`,
`notes.Delete`, `sms.Send` and the rest. The test is an irreversible effect
nobody asked for: the agent reads text strangers wrote, so a tool it holds is a
tool prompt injection holds. Deleting your own file from the page is fine;
having it deleted because a web page said so is not.

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
5. If a method changes anything — creates, updates, sends, stores, pays — set
   `Writes: true` on it, so the HTTP door refuses to perform it on a GET.
6. If a method is irreversible and should only follow from a user's own action,
   set `Destructive: true` on it as well, which also withholds it from the
   model.
7. Add a row above.

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

### Docs — `mu.db` (collections, private/public)

Named collections of JSON records. Every record has a **server-set owner** (the
signed-in user) and a **public** flag, so one app can hold each user's private
data plus a shared public set. This is the building block for real apps — notes,
lists, posts, trackers — where "mine" and "public" both matter.

This is the record store, and it has no page and no tools of its own — "a
database" is not a kind of thing a person makes, it is how all the kinds are
stored. `service/docs` used to expose it directly, which is why writing a
document meant typing JSON into a form; Docs is now documents (a title and a
markdown body) and this stays underneath, reached by apps as `mu.db`.

Each app has its own namespace, so what one app writes is not what another
reads, and neither is what the caller's documents hold. A record published with
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

Owner scoping and the private/public model are the same wherever this store is
used. `service/docs` builds on it — a document is a record with a title and a
body — but its tools are `docs_write` / `docs_read` / `docs_list` /
`docs_delete`, which speak documents rather than records.

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

## Layering

Four levels, and everything points down.

```
              ┌───────────────────────────────────────────────┐
   PROGRAMS   │   internal/server        internal/cli         │
              │   assemble everything, so they import         │
              │   everything — this is where the wiring lives │
              └───────────────────────┬───────────────────────┘
                                      │ constructs and wires
   ═══════════════════════════════════▼═══════════════════════════  the product
              ┌───────────┬───────────┬───────────┬───────────┐
   DOORS      │  home/    │  admin/   │ account/  │           │
              │  the web  │  Discord  │  ops and  │ who you   │
              │  UI       │  Telegram │  moder-   │ are, what │
              │           │  WhatsApp │  ation    │ you can   │
              │           │           │           │ afford    │
              └─────┬─────┴─────┬─────┴─────┬─────┴───────────┘
                    │           │           │
                    └───────────┼───────────┘
                                ▼
              ┌───────────────────────────────────────────────┐
   DECIDERS   │  agent/    micro/  blog/  social/             │
              │            digest/                            │
              │  Takes a goal, reads the catalogue, chooses    │
              │  which questions to ask. Cannot be in the      │
              │  catalogue, because it consumes it.           │
              └───────────────────────┬───────────────────────┘
                                      │ calls tools
              ┌───────────────────────▼───────────────────────┐
   ANSWERERS  │  service/  — 31 of them, no edges between any  │
              │  two. Request in, response out, deterministic  │
              │  given the data. A tool is derived from one;   │
              │  tool/ is that derivation and nothing else.    │
              └───────────────────────┬───────────────────────┘
                                      │ may import freely
   ═══════════════════════════════════▼═══════════════════════════  the substrate
              ┌───────────────────────────────────────────────┐
   SUBSTRATE  │  internal/ — app auth data quota ai settings   │
              │  x402 service api event blob gtfs phone …      │
              │  Nothing here has a name a user would          │
              │  recognise, and nothing here may look up.      │
              └───────────────────────────────────────────────┘
```

Six rules follow from the picture.

1. **Down only.** `internal/` is the runtime, not the features, and it may never
   import the product. The two exceptions are the programs: `internal/server`
   and `internal/cli` assemble everything, so they import everything
2. **Services import the substrate freely** — that's what it's for
3. **Services must not import each other.** Whatever two of them share goes in
   `internal/`. A sideways import makes two services one unit: read together,
   changed together, moved together, and the catalogue stops being a list of
   independent things
4. **An agent may import a service; a service may never import an agent.** This
   is rule 3 stated for the level above, and it is the one with a reason rather
   than a convention behind it. A service answers a question about state; an
   agent decides which question to ask. A service that calls an agent is asking
   the model what its own answer should be
5. **No service asks what money is.** A service declares what an operation costs
   (`Cost: quota.OpWebSearch` on its `Endpoint`) and `internal/quota` answers.
   Quota holds prices and deliberately does not know what a balance is;
   `account/` fills in that half from its own init, because quota sits
   underneath it. There used to be a cross-cutting `wallet` every service
   imported, and this is what replaced it
6. **`admin` imports the product and nothing imports `admin`.** It is a
   management UI over the services — `mail` for the spam filter, `account` for
   the ledger, `apps`, `news` and `markets` for the panel's own views. An
   acceptable coupling because it is a leaf

Rules 1, 3 and 5 are enforced by `test/layering_test.go`. Rules 2, 4 and 6 are
conventions, and rule 4 is the one currently broken — see below.

## Where it leaks

The picture above is what the code mostly is. What follows is where it is not,
written down because two of these hid for a year inside tests whose whole
subject was catching them.

**A rule you can get out of by making a subdirectory.** Both layering tests
stopped at the first directory level. `TestServicesDoNotImportEachOther` globbed
`service/<name>/*.go`, and its pattern ended at the closing quote so
`"mu/service/markets"` matched but `"mu/service/news/digest"` did not look like
a service import at all. `TestInternalNeverImportsTheProduct` matched `"mu/agent"`
and missed `"mu/agent/micro"`. Two real violations lived in exactly that gap:
The since-deleted `internal/a2a` imported the micro agent, and `service/news/digest` imported
`markets` and `video`. Both tests now walk the whole subtree, and both
violations are fixed — `digest` is under `agent/`, where the imports
they need are legal.

**`internal/server/hooks.go` is the bill.** Around 880 lines and 47 function
variables, each one a dependency somebody could not express as an import. They
are not one thing, and the file reads as though they were:

| What it is | Roughly | Verdict |
|---|---|---|
| `internal/` needing something from the product — `app.EmailSender`, `auth.HasCredit`, `api.WalletPayer`, `profile.GetUserPosts`, `quota.LimitOverride`, `service.Gate.*` | 22 | **Correct.** Rule 1 leaves no alternative, and this is what the exception costs |
| A service needing another service — `news.FetchSocialContext` | 1 | **Rule 3 in letter only.** There is no import, and the two packages still change together. `email.SendVia` went with the email service |
| A service announcing something happened — `events.OnCreate`, `events.OnFire` | 2 | **Right direction, wrong shape.** Outward rather than upward, so not a leak — but a hook takes exactly one listener and `internal/event` takes any number |
| `mail.OnNewMail` | 0 | **Gone.** Mail arriving is a fact, not a call: `service/mail` publishes `event.MailReceived` and knows nothing about who listens. The pattern the other four should follow |
| A service needing the agent | 0 | **Gone.** `tasks.RunAgent`, `events.RunAgent` and `events.OnFireEvent` were three; a fourth, `stream.AIReplyHook`, went earlier. Asking an agent for work is a fact now — `event.WorkForAgent`, published by the service holding the record and subscribed by `agent/work`. Same inversion as `mail.OnNewMail` above |
| A service needing the money — `apps.QuotaCheck`, `apps.ChargeQuota`, `apps.ChargeUse` | 3 | **Was filed as correct; it is not.** Rule 5 says a service asks `internal/quota` what an operation costs, and `TestNoServiceImportsTheAccount` asserts zero imports and passes — these are how that is avoided. Two of the three are metering, which belongs at the door and not in the service; the third pays an app's author out of a payer's balance, which quota genuinely cannot express. That is a gap in quota, not a licence here |
| A service needing Google — `events.External*`, `contacts.External*` | 6 | **Not debt at all.** `internal/google` imports only `data` and `settings`; either service could import it directly under rule 2. The indirection buys provider-neutrality, which is a design choice and not a layering one |

Rows two, four and five are the ones worth acting on. The first is the price of
rule 1 working, and the last is a design choice rather than a layering one.

**A function variable is an import the compiler cannot see.** That is what all
of this has in common, and it is why the table was wrong for a year: every
layering test here reads import statements, and every edge they forbid has been
made anyway as a `var X func(...)` that `hooks.go` fills in at boot. The import
is gone, the dependency is not, and the test reports clean.
`test/service_hooks_test.go` counts them instead — it pins the list, classifies
each one, and fails when the number goes up or when the ledger stops matching
the tree. Both had already happened: this table named `stream.AIReplyHook` and
`email.SendVia` long after both were deleted.

One of the second row is already gone, and it is worth saying which and why.
`mail.KnownSender` was wired to `service/contacts` so mail could ask whether a
sender is in the address book — a hook justified by rule 3. But the address book
*is* `internal/contacts`; `service/contacts` is the tools, the page and the
Google bridge over it. Rule 3 was never in the way and rule 2 always allowed the
import. Before reaching for a hook, check whether the thing being reached for
already has a home in the substrate.

The agent row is empty because that inversion has been done. It was three
hooks, and counting them is what showed they were one thing: four doors ask an
agent for work — a chat message, mail arriving, a task assigned, a schedule
firing — and three of them reached upward. The services announce now.

What is left is `news.FetchSocialContext`, which wants another service and
belongs in `internal/`, and the three `apps` hooks, of which two are metering
that belongs at the door and one is a transfer `internal/quota` cannot
express.

**One door for a tool a model named.** Two questions have to be asked before a
model's chosen tool runs — may a caller with no account use it, and is it one of
the destructive ones withheld from the model — and they were asked by the
callers, written out above each execution site. The arithmetic never held: the
destructive check was at one site and missing at the next for as long as
`agent/micro` existed, and the residual guest allowlist was copied into two
files that then drifted. The guards now live in `api.RunPlanned` and
`api.RunPlannedAs`, nothing under `agent/` may call `api.ExecuteTool` with a
name it did not write itself, and `test/destructive_test.go` checks that rather
than checking each site for a nearby guard clause. A fifth execution site gets
the answers whether or not its author knew to ask.

**Not a leak: `tool/` at the top level.** It is 280 lines deriving `api.Tool`
from `service.Spec` and it imports nothing but `internal/`, which reads like
something that belongs underneath. It does not. The top level is the sidebar —
Home, Account, Tools, Agents, Services — and Tools is one of them, so it has a
directory for the same reason `home/` does. What sits under it is small because
a tool is derived rather than written, which is the design working.
