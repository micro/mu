# Service registry

What exists today and how the pieces differ. For what to build next, see
[SERVICES.md](SERVICES.md).

A service is one capability. Almost everything else in Mu derives from the set
of registered services: the agent's tools, the `/agent/new` picker, what apps
can call through the SDK, and the status page. Register a service and those
surfaces pick it up with no further wiring.

Each service lives in its own directory under `service/`, named after it, and registers
with `service.Register("name", new(Server))`. The handler is plain typed Go —
`func (Server) Method(ctx, *Req, *Rsp) error` — and the `description` struct
tags on request and response fields become the tool schema. There is no
manifest file; the types are the manifest.

## The convention

**service name == directory == route == nav label == tool prefix**

Services live under `service/<name>/`. `internal/service` is the runtime core
that hosts them — it is not itself a service.

The exception is a **headless** service: a capability with no page, so no route
and no nav entry. `index`, `web` and `db` are headless. `wallet` has a page that
predates its service; both are the same capability with two surfaces.

## What is registered

| Service | Page | Agent tool | Account-scoped | What it is |
|---|---|---|---|---|
| `news` | `/news` | ✅ | | RSS aggregation, sentiment, search |
| `blog` | `/blog` | ✅ | | Microblogging, daily digests, ActivityPub |
| `mail` | `/mail` | ✅ | ✅ | SMTP server, inbox, DKIM |
| `markets` | `/markets` | ✅ | | Crypto, futures, commodities, currencies |
| `weather` | `/weather` | ✅ | | Forecast and pollen |
| `places` | `/places` | ✅ | | Maps and points of interest |
| `video` | `/video` | ✅ | | Search and playback |
| `social` | `/social` | ✅ | | Threads, replies, status |
| `search` | `/search` | ✅ | | Web search |
| `images` | `/images` | ✅ | ✅ | Generation, daily image, archive |
| `events` | `/events` | ✅ | ✅ | Scheduled reminders, `.ics` invites |
| `islam` | `/islam` | ✅ | | Daily reminder, prayer times, qibla |
| `apps` | `/apps` | ✅ | | User apps: build, run, edit |
| `index` | — | ✅ | ✅ | Search across the caller's own content |
| `web` | — | ✅ | | Fetch a URL, return readable content |
| `db` | — | ✅ | ✅ | Per-user records, for services and apps |
| `wallet` | `/wallet` | ✅ | ✅ | Credit check, charge, balance |

## Account-scoped

Listed in `internal/service/dynamic.go`. These hold data belonging to one user
or spend their credits, so a caller with no authenticated account cannot reach
them at all.

Identity comes from the **call context**, never from a request field — see
`internal/service/identity.go`. `CallDynamic` discards any `account_id` a caller
supplies and re-stamps it from the context, so no caller can scope a call to
someone else by naming them.

## Destructive methods

**Every registered service becomes an agent tool.** That is the point of
deriving from the registry — register a service and the model can use it.

The guard is per *method*, not per service (`destructiveTools` in
`agent/native.go`). Two are withheld:

- **`wallet.charge`** — spending should follow from the user's own action
- **`db.delete`** — irreversible, and the user can delete from the app

Everything else on those services is available: the agent can read a balance,
check a cost, and create, list, get and update records.

The reasoning is not that the services are dangerous. The agent reads
attacker-controlled text — an email body, a page it just fetched — so any tool
it holds is a tool prompt injection holds. What earns a place on that list is an
irreversible side effect nobody asked for. Note the agent *already* spends
credits: generating an image costs 15. Withholding a whole service to protect
one method would be both too blunt and inconsistent with that.

A blocked call is refused before it runs and the model is told why, so it can
explain rather than retry.

## Adding one

1. Create `service/<name>/`, named for the service.
2. Write `Server` with typed methods and `description` tags.
3. `service.Register("name", new(Server))` from a `Load()`, called in `main.go`.
4. If it holds per-user data or spends credits, add it to `accountScoped`.
5. If a method is irreversible and should only follow from a user's own
   action, add it to `destructiveTools` in `agent/native.go`.
6. Add a row above.

Nothing else is needed. The agent, the picker, the app SDK and the status page
all read the registry.

One thing that does **not** derive yet: MCP tools are still hand-written in
`internal/api/mcp.go`, so a new service is an agent tool but not an MCP tool
until someone adds a stanza. See micro/mu#1445.
